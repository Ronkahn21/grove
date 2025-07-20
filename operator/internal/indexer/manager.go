package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// IndexManager manages pod indices for standalone PodCliques.
// Handles holes, out-of-range indices, duplicates, and missing hostnames gracefully.
// Prioritizes filling lower indices first.
type IndexManager struct {
	biMap *bidirectionalMap
}

// InitializationError represents an error that occurred during IndexManager initialization
type InitializationError struct {
	PodName string
	Reason  string
	Err     error
}

func (e InitializationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("pod %s: %s: %v", e.PodName, e.Reason, e.Err)
	}
	return fmt.Sprintf("pod %s: %s", e.PodName, e.Reason)
}

type bidirectionalMap struct {
	podToIndex  map[types.UID]int
	indexToPod  map[int]types.UID
	replicaSize int
}

func newBidirectionalMap(replicaSize int) *bidirectionalMap {
	return &bidirectionalMap{
		podToIndex:  make(map[types.UID]int),
		indexToPod:  make(map[int]types.UID),
		replicaSize: replicaSize,
	}
}

func (b *bidirectionalMap) assignIndex(podUID types.UID, index int) {
	// Remove any existing assignment for this pod
	if existingIndex, exists := b.podToIndex[podUID]; exists {
		delete(b.indexToPod, existingIndex)
	}

	// Remove any existing pod at this index
	if existingPod, exists := b.indexToPod[index]; exists {
		delete(b.podToIndex, existingPod)
	}

	// Make new assignment
	b.podToIndex[podUID] = index
	b.indexToPod[index] = podUID
}

func (b *bidirectionalMap) getPodIndex(podUID types.UID) (int, bool) {
	index, exists := b.podToIndex[podUID]
	return index, exists
}

func (b *bidirectionalMap) isIndexUsed(index int) bool {
	_, exists := b.indexToPod[index]
	return exists
}

func (b *bidirectionalMap) getAvailableIndex() int {
	// Find lowest available index within replica size
	for i := 0; i < b.replicaSize; i++ {
		if !b.isIndexUsed(i) {
			return i
		}
	}

	// All indices within replica size are used, find next available
	nextIndex := b.replicaSize
	for b.isIndexUsed(nextIndex) {
		nextIndex++
	}
	return nextIndex
}

func NewIndexManager(replicaSize int, existingPods []*corev1.Pod) (*IndexManager, []InitializationError) {
	im := &IndexManager{
		biMap: newBidirectionalMap(replicaSize),
	}

	var errors []InitializationError

	for _, pod := range existingPods {
		if pod == nil {
			continue
		}

		index, err := extractIndexFromHostname(pod.Spec.Hostname)
		if err != nil {
			errors = append(errors, InitializationError{
				PodName: pod.Name,
				Reason:  "invalid hostname",
				Err:     err,
			})
			continue
		}

		if im.biMap.isIndexUsed(index) {
			errors = append(errors, InitializationError{
				PodName: pod.Name,
				Reason:  fmt.Sprintf("duplicate index %d", index),
			})
			continue
		}

		im.biMap.assignIndex(pod.UID, index)
	}

	return im, errors
}

func (im *IndexManager) GetIndex(pod *corev1.Pod) (int, error) {
	if pod == nil {
		return -1, errors.New("pod is nil")
	}

	// Return existing index if pod is already known
	if existingIndex, exists := im.biMap.getPodIndex(pod.UID); exists {
		return existingIndex, nil
	}

	// Assign new index
	newIndex := im.biMap.getAvailableIndex()
	im.biMap.assignIndex(pod.UID, newIndex)

	return newIndex, nil
}

func extractIndexFromHostname(hostname string) (int, error) {
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
