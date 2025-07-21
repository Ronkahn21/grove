package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type bidirectionalMap struct {
	podToIndex  map[string]int
	indexToPod  map[int]string
	replicaSize int
}

func newBidirectionalMap(replicaSize int) *bidirectionalMap {
	return &bidirectionalMap{
		podToIndex:  make(map[string]int),
		indexToPod:  make(map[int]string),
		replicaSize: replicaSize,
	}
}

func (b *bidirectionalMap) assignIndex(podName string, index int) {
	// Remove any existing assignment for this pod
	if existingIndex, exists := b.podToIndex[podName]; exists {
		delete(b.indexToPod, existingIndex)
	}

	// Remove any existing pod at this index
	if existingPod, exists := b.indexToPod[index]; exists {
		delete(b.podToIndex, existingPod)
	}

	// Make new assignment
	b.podToIndex[podName] = index
	b.indexToPod[index] = podName
}

func (b *bidirectionalMap) getPodIndex(podName string) (int, bool) {
	index, exists := b.podToIndex[podName]
	return index, exists
}

func (b *bidirectionalMap) isIndexUsed(index int) bool {
	_, exists := b.indexToPod[index]
	return exists
}

func (b *bidirectionalMap) getAvailableIndex() (int, error) {
	// Find lowest available index within replica size
	for i := 0; i < b.replicaSize; i++ {
		if !b.isIndexUsed(i) {
			return i, nil
		}
	}

	// All indices within replica size are used - this is an error
	return -1, errors.New("no available index within replica size")
}

// IndexManager manages pod indices for PodCliques.
// Prioritizes filling lower indices first.
type IndexManager struct {
	biMap *bidirectionalMap
}

func NewIndexManager(replicaSize int, podIndexMappings map[string]int) *IndexManager {
	im := &IndexManager{
		biMap: newBidirectionalMap(replicaSize),
	}

	for podName, index := range podIndexMappings {
		im.biMap.assignIndex(podName, index)
	}

	return im
}

func (im *IndexManager) GetIndex(pod *corev1.Pod) (int, error) {
	if pod == nil {
		return -1, errors.New("pod is nil")
	}

	// Return existing index if pod is already known
	if existingIndex, exists := im.biMap.getPodIndex(pod.Name); exists {
		return existingIndex, nil
	}

	// Assign new index
	newIndex, err := im.biMap.getAvailableIndex()
	if err != nil {
		return -1, fmt.Errorf("failed to assign index to pod %s: %w", pod.Name, err)
	}
	im.biMap.assignIndex(pod.Name, newIndex)

	return newIndex, nil
}

func ExtractIndexFromHostname(hostname string) (int, error) {
	if hostname == "" {
		return -1, errors.New("hostname is empty")
	}

	parts := strings.Split(hostname, "-")
	if len(parts) < 2 {
		return -1, errors.New("hostname does not contain index suffix")
	}

	indexStr := parts[len(parts)-1]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return -1, fmt.Errorf("failed to parse index from hostname suffix '%s': %w", indexStr, err)
	}

	return index, nil
}

func InitIndexManger(existingPods []*corev1.Pod, expectedPodsSize int, logger logr.Logger) *IndexManager {
	im := &IndexManager{
		biMap: newBidirectionalMap(expectedPodsSize),
	}

	// Process pods in order to maintain "last wins" behavior for duplicate indices
	for _, pod := range existingPods {
		if pod == nil {
			continue
		}
		if pod.Spec.Hostname == "" {
			continue
		}
		index, err := ExtractIndexFromHostname(pod.Spec.Hostname)
		// for now, we ignore the error and continue processing other pods
		// in the future we might want to handle this differently
		// e.g. by marking deleted pods and creating new pods
		// with valid indexes
		if err != nil {
			logger.Error(err, "Failed to extract index from pod hostname",
				"podName", pod.Name, "podHostname", pod.Spec.Hostname)
			continue
		}
		if index < 0 || index >= expectedPodsSize {
			logger.Error(nil, "Extracted index is out of bounds for PodClique replicas",
				"podName", pod.Name, "podHostname", pod.Spec.Hostname,
				"index", index, "replicas", expectedPodsSize)
			continue
		}
		im.biMap.assignIndex(pod.Name, index)
	}
	return im
}
