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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName={td}

// TopologyDomain defines the topology hierarchy for the cluster.
// This resource is immutable after creation.
// Only one TopologyDomain can exist cluster-wide (enforced by webhook).
type TopologyDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the topology hierarchy specification.
	Spec TopologyDomainSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TopologyDomainList is a list of TopologyDomain resources.
type TopologyDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of TopologyDomains.
	Items []TopologyDomain `json:"items"`
}

// TopologyDomainSpec defines the topology hierarchy specification.
type TopologyDomainSpec struct {
	// Levels is an ordered list of topology levels from broadest to narrowest scope.
	// The order in this list defines the hierarchy (index 0 = highest level).
	// This field is immutable after creation.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="levels list is immutable"
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Levels []TopologyLevel `json:"levels"`
}

// TopologyLevel defines a single level in the topology hierarchy.
type TopologyLevel struct {
	// Name is the level identifier used in TopologyConstraint references.
	// Must be a valid DNS label (lowercase alphanumeric with hyphens).
	// Examples: "zone", "rack", "host"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// TopologyKey is the node label key that identifies this topology domain.
	// Must be a valid Kubernetes label key (qualified name).
	// Examples: "topology.kubernetes.io/zone", "kubernetes.io/hostname"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	TopologyKey string `json:"topologyKey"`

	// Description provides human-readable information about this level.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Description string `json:"description,omitempty"`
}
