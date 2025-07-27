// /*
// Copyright 2025 The Grove Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// GetNextAvailableIndices returns the next N available indices for pods.
// Extracts indices from existing pod hostnames and returns count available indices,
// filling holes from lowest to highest (starting from 0).
func GetNextAvailableIndices(existingPods []*corev1.Pod, requiredIndicesCount int, logger logr.Logger) []int {
	usedIndices := extractUsedIndices(existingPods, logger)
	return findAvailableIndices(&usedIndices, requiredIndicesCount)
}

// extractUsedIndices extracts and validates indices from existing pods.
func extractUsedIndices(existingPods []*corev1.Pod, logger logr.Logger) sets.Set[int] {
	usedIndices := sets.New[int]()

	for _, pod := range existingPods {
		if pod == nil {
			continue
		}

		index, err := extractIndexFromHostname(pod.Spec.Hostname)
		if err != nil {
			logger.Error(err, "failed to extract index from hostname",
				"pod", pod.Name, "hostname", pod.Spec.Hostname)
			continue
		}

		if usedIndices.Has(index) {
			logger.Error(fmt.Errorf("duplicate index %d", index),
				"duplicate index found", "pod", pod.Name, "hostname", pod.Spec.Hostname)
			continue
		}

		usedIndices.Insert(index)
	}

	return usedIndices
}

// isValidIndex checks if an index is valid (non-negative).
func isValidIndex(index int) bool {
	return index >= 0
}

// findAvailableIndices finds the next count available indices starting from 0.
func findAvailableIndices(usedIndices *sets.Set[int], requiredIndicesCount int) []int {
	availableIndices := make([]int, requiredIndicesCount)
	currentIndex := 0
	foundIndexCounter := 0

	for foundIndexCounter < requiredIndicesCount {
		if !usedIndices.Has(currentIndex) {
			availableIndices[foundIndexCounter] = currentIndex
			usedIndices.Insert(currentIndex)
			foundIndexCounter++
		}
		currentIndex++
	}

	return availableIndices
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

	if index < 0 {
		return -1, fmt.Errorf("extracted index %d is negative", index)
	}

	return index, nil
}
