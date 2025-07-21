package indexer

import (
	"container/list"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type indexTracker struct {
	podToIndex        map[string]int
	usedIndices       *list.List
	highestContinuous *list.Element
}

func newIndexTracker(indexedPods map[string]int) *indexTracker {
	tracker := &indexTracker{
		podToIndex:        indexedPods,
		usedIndices:       list.New(),
		highestContinuous: nil,
	}

	indexArray := slices.Collect(maps.Values(indexedPods))
	slices.Sort(indexArray)
	// Add indices to usedIndices list in sorted order
	for _, index := range indexArray {
		tracker.usedIndices.PushBack(index)
	}
	// Initialize highestContinuous
	tracker.findZeroFirstHighestContinuous()

	return tracker
}

func (it *indexTracker) GetPodIndex(podName string) (int, bool) {
	index, exists := it.podToIndex[podName]
	return index, exists
}

func (it *indexTracker) AssignAvailableIndex(podName string) int {
	if index, exists := it.GetPodIndex(podName); exists {
		return index // Pod already has an index assigned
	}
	defer it.findZeroFirstHighestContinuous()

	if it.usedIndices.Len() == 0 || it.highestContinuous == nil {
		return it.insertInitIndexToFront(podName)
	}
	return it.insertAfterHighestContinues(podName)
}

func (it *indexTracker) insertAfterHighestContinues(podName string) int {
	nextIndex := it.highestContinuous.Value.(int) + 1
	// insert the pod after the next highest continuous index
	it.usedIndices.InsertAfter(nextIndex, it.highestContinuous)
	it.podToIndex[podName] = nextIndex
	return nextIndex
}

func (it *indexTracker) insertInitIndexToFront(podName string) int {
	it.highestContinuous = it.usedIndices.PushFront(0)
	it.podToIndex[podName] = 0
	return 0 // First pod gets index 0
}

func (it *indexTracker) findZeroFirstHighestContinuous() {
	// handle case where first index is not 0
	first := it.usedIndices.Front()
	if isElementZero(first) {
		return
	}
	// If first element is 0, we need to find the end of the continuous sequence
	if it.highestContinuous != nil {
		first = it.highestContinuous
	}
	current := findNewHighestContinuesPointer(first)

	it.highestContinuous = current
}

func isElementZero(first *list.Element) bool {
	return first == nil || first.Value.(int) != 0
}

func findNewHighestContinuesPointer(element *list.Element) *list.Element {
	// Find end of continuous sequence starting from 0
	current := element
	for current != nil {
		next := current.Next()
		if next == nil || next.Value.(int) != current.Value.(int)+1 {
			break
		}
		current = next
	}
	return current
}

// IndexManager manages pod indices for PodCliques.
// Prioritizes filling lower indices first.
type IndexManager struct {
	tracker *indexTracker
}

// NewIndexManager creates a new IndexManager with pre-existing pod index mappings.
func NewIndexManager(podIndexMappings map[string]int) *IndexManager {
	return &IndexManager{
		tracker: newIndexTracker(podIndexMappings),
	}
}

// GetIndex returns the index for the given pod, assigning a new one if needed.
func (im *IndexManager) GetIndex(pod *corev1.Pod) (int, error) {
	if pod == nil {
		return -1, errors.New("pod is nil")
	}

	// Return existing index if pod is already known
	if existingIndex, exists := im.tracker.GetPodIndex(pod.Name); exists {
		return existingIndex, nil
	}

	// Assign new index
	newIndex := im.tracker.AssignAvailableIndex(pod.Name)

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
func InitIndexManger(existingPods []*corev1.Pod, logger logr.Logger) *IndexManager {
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

	return NewIndexManager(indexedPods)
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
