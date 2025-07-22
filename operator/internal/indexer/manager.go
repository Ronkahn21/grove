package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/grove/operator/internal/bimaps"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// IndexManagerStats contains statistics about the index manager state.
type IndexManagerStats struct {
	HighestIndex     int // Highest continuous index starting from 0
	MaxIndexAssigned int // Maximum index that has been assigned
	UsedIndices      int // Total number of indices in use
}

// IndexManager manages pod indices for PodCliques.
// Prioritizes filling lower indices first by maintaining a continuous sequence from 0.
type IndexManager struct {
	podIndexMap  *bimaps.BiMap[string, int] // Bidirectional pod name ↔ index mapping
	highestIndex int                        // Highest continuous index starting from 0
	maxAssigned  int                        // Maximum index that has been assigned
}

// NewIndexManager creates a new IndexManager with pre-existing pod index mappings.
func NewIndexManager(podIndexMappings map[string]int, maxAssigned int) *IndexManager {
	manager := &IndexManager{
		podIndexMap:  bimaps.New[string, int](),
		highestIndex: -1, // Initialize to -1 to indicate no indices assigned yet
		maxAssigned:  maxAssigned,
	}

	// Populate the BiMap with existing mappings
	for podName, index := range podIndexMappings {
		manager.podIndexMap.Set(podName, index)
	}

	// Calculate the highest continuous index starting from 0
	manager.updateHighestContinuousIndex()
	return manager
}

// GetPodIndex returns the index for the given pod name if it exists.
func (im *IndexManager) GetPodIndex(podName string) (int, bool) {
	return im.podIndexMap.GetByKey(podName)
}

// assignAvailableIndex assigns the next available index to a pod.
func (im *IndexManager) assignAvailableIndex(podName string) int {
	if index, exists := im.GetPodIndex(podName); exists {
		return index // Pod already has an index assigned
	}
	defer im.updateHighestContinuousIndex()

	var nextIndex int
	if im.highestIndex == -1 {
		// No indices assigned yet, start with 0
		nextIndex = 0
	} else {
		nextIndex = im.highestIndex + 1
	}

	im.podIndexMap.Set(podName, nextIndex)
	return nextIndex
}

// updateHighestContinuousIndex updates the highest continuous index starting from 0.
func (im *IndexManager) updateHighestContinuousIndex() {
	if !im.podIndexMap.HasValue(0) {
		im.highestIndex = -1
		return
	}

	// Start from 0 and increment until we find a gap or reach maxAssigned
	startIndex := 0
	if im.highestIndex != -1 {
		startIndex = im.highestIndex
	}
	for i := startIndex; i < im.maxAssigned; i++ {
		if !im.podIndexMap.HasValue(i) {
			break
		}
		im.highestIndex = i
	}
}

// GetIndex returns the index for the given pod, assigning a new one if needed.
func (im *IndexManager) GetIndex(pod *corev1.Pod) (int, error) {
	if pod == nil {
		return -1, errors.New("pod is nil")
	}

	// Return existing index if pod is already known
	if existingIndex, exists := im.GetPodIndex(pod.Name); exists {
		return existingIndex, nil
	}

	// Assign new index
	newIndex := im.assignAvailableIndex(pod.Name)

	return newIndex, nil
}

// extractIndexFromHostname extracts the numeric index from a pod hostname.
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

// InitIndexManger creates a new IndexManager initialized with existing pods.
func InitIndexManger(existingPods []*corev1.Pod, maxAssigned int, logger logr.Logger) *IndexManager {
	indexedPods := make(map[string]int, len(existingPods))
	usedIndices := sets.New[int]()

	for _, pod := range existingPods {
		if pod == nil {
			continue
		}
		index, err := extractIndexFromHostname(pod.Spec.Hostname)
		if validationErr := validateHostNameIndex(err, pod, index, indexedPods, usedIndices); validationErr != nil {
			logger.Error(validationErr, "invalid hostname index",
				"pod", pod.Name, "hostname", pod.Spec.Hostname)
			continue
		}

		indexedPods[pod.Name] = index
		usedIndices.Insert(index)
	}

	return NewIndexManager(indexedPods, maxAssigned)
}

// validateExtractionError checks if index extraction from hostname failed
func validateExtractionError(err error, hostname string) error {
	if err != nil {
		return fmt.Errorf("failed to extract index from pod hostname '%s': %w", hostname, err)
	}
	return nil
}

// validateIndexValue checks if the extracted index value is valid
func validateIndexValue(index int, hostname string) error {
	if index < 0 {
		return fmt.Errorf("extracted index %d from pod hostname '%s' is negative", index, hostname)
	}
	return nil
}

// validateIndexUniqueness checks if the index is already used by another pod
func validateIndexUniqueness(index int, hostname string, usedIndex sets.Set[int]) error {
	if usedIndex.Has(index) {
		return fmt.Errorf("pod hostname '%s' has duplicate index %d", hostname, index)
	}
	return nil
}

// validatePodUniqueness checks if the pod name is already indexed
func validatePodUniqueness(podName string, hostname string, indexedPods map[string]int) error {
	if _, exists := indexedPods[podName]; exists {
		return fmt.Errorf("pod hostname '%s' is already indexed, duplicate", hostname)
	}
	return nil
}

func validateHostNameIndex(err error, pod *corev1.Pod, index int,
	indexedPods map[string]int, usedIndex sets.Set[int]) error {
	hostname := pod.Spec.Hostname

	// Chain validation checks - return first error encountered
	validators := []func() error{
		func() error { return validateExtractionError(err, hostname) },
		func() error { return validateIndexValue(index, hostname) },
		func() error { return validateIndexUniqueness(index, hostname, usedIndex) },
		func() error { return validatePodUniqueness(pod.Name, hostname, indexedPods) },
	}

	for _, validate := range validators {
		if validationErr := validate(); validationErr != nil {
			return validationErr
		}
	}
	return nil
}
