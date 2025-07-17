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

package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// GetPodGangSetReplicaIndexFromPodCliqueFQN extracts the PodGangSet replica index from a Pod Clique FQN name.
func GetPodGangSetReplicaIndexFromPodCliqueFQN(pgsName, pclqFQNName string) (int, error) {
	replicaStartIndex := len(pgsName) + 1 // +1 for the hyphen
	hyphenIndex := strings.Index(pclqFQNName[replicaStartIndex:], "-")
	if hyphenIndex == -1 {
		return -1, fmt.Errorf("PodClique FQN is not in the expected format of <pgs-name>-<pgs-replica-index>-<pclq-template-name>: %s", pclqFQNName)
	}
	replicaEndIndex := replicaStartIndex + hyphenIndex
	return strconv.Atoi(pclqFQNName[replicaStartIndex:replicaEndIndex])
}

// GetPodCliqueReplicaIndexFromPodCliqueFQN extracts the PodClique replica index within PCSG from a Pod Clique FQN name.
// For PCSG PodCliques, this currently returns 0 since each PodClique template gets one replica per PCSG.
// The index represents the replica index of the PodClique within the PCSG context.
func GetPodCliqueReplicaIndexFromPodCliqueFQN(pgsName, pclqFQNName string) (int, error) {
	// Format: {pgs-name}-{pgs-replica-index}-{pclq-template-name}
	// For now, PodClique replica index within PCSG is always 0 since each template gets one replica
	// This function is added for future extensibility when PCSG might have multiple replicas of the same PodClique template
	_, err := GetPodGangSetReplicaIndexFromPodCliqueFQN(pgsName, pclqFQNName)
	if err != nil {
		return -1, fmt.Errorf("failed to validate PodClique FQN format: %w", err)
	}
	return 0, nil
}

// GetPCSGReplicaIndexFromPCSGFQN extracts the PCSG replica index within PodGangSet from a PCSG FQN name.
// Format: {pgs-name}-{pgs-replica-index}-{pcsg-name}
func GetPCSGReplicaIndexFromPCSGFQN(pgsName, pcsgFQNName string) (int, error) {
	replicaStartIndex := len(pgsName) + 1 // +1 for the hyphen
	hyphenIndex := strings.Index(pcsgFQNName[replicaStartIndex:], "-")
	if hyphenIndex == -1 {
		return -1, fmt.Errorf("PCSG FQN is not in the expected format of <pgs-name>-<pgs-replica-index>-<pcsg-name>: %s", pcsgFQNName)
	}
	replicaEndIndex := replicaStartIndex + hyphenIndex
	return strconv.Atoi(pcsgFQNName[replicaStartIndex:replicaEndIndex])
}
