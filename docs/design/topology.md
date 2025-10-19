# Topology-Aware Scheduling - Grove Operator Design

## Overview

This document defines the design for implementing topology-aware scheduling in the Grove operator.

**Motivation for Grove**: Topology-aware scheduling is critical for Grove's multinode inference workloads because these
applications require:

- **Network Locality**: High-bandwidth communication between prefill and decode workers benefits from proximity
- **Coordinated Placement**: Related components (e.g., model shards) perform better when co-located within the same
  topology domain
- **Latency Optimization**: Minimizing network hops between interdependent inference components improves end-to-end
  performance

## Topology Architecture Overview

Grove implements comprehensive topology-aware scheduling across all scalable resources using a domain-based packing
constraint model:

- **Universal Resource Support**: All Grove resources with scale sub-resource (PodCliqueSet, PodCliqueScalingGroup,
  PodClique) support topology constraints with hierarchical validation

- **Domain-Based Packing**: Grove uses domain references to specify where replicas should be packed together for optimal
  network locality

## Requirements

### Core Resource Requirements

- **All Grove Scale Resources**: All Grove resources with scale sub-resource (PodCliqueSet, PodCliqueScalingGroup,
  PodClique) should support topology packing constraints
- **Hierarchical Constraints**: Topology constraints for child resources should be equal to or stricter than parent
  constraints
    - Example: If PCS replica must be within zone, PCSG/PCLQ constraints should be zone or stricter (rack, host)
- **Packing Support**: Each resource should support packing replicas within specified topology domains for network
  locality

### Constraint Model Requirements

- **Single-Domain Model**: Support PackDomain to specify where replicas should be packed together
- **Required Constraints Only**: Only required topology constraints with automatic fallback to less strict levels when
  preferred constraints cannot be satisfied
- **Dynamic Domain Hierarchy**: Domain hierarchy defined via TopologyDomain CRD with list-based ordering
- **Preferred Defaults**: Preferred pack constraints default to strictest level (host) with automatic fallback

### User Experience Requirements

- **Non-Power Users**: Should get automatic topology optimization with no configuration needed when admin configures
  default topology
- **Power Users**: Should be able to specify topology packing constraints for fine-grained control
- **Configuration Consistency**: If topology is not configured, no topology features should be applied regardless of
  user specifications

### Technical Requirements

- **Immutability**: All topology configuration must be immutable after resource creation
- **Admin Configuration**: Admin should configure default topology for the grove operator
- **Automatic Population**: Mutation webhook should autopopulate admin defaults if not provided
- **Constraint Validation**: Validate domain hierarchy rules using index-based comparison
- **Topology Deletion Prevention**: Prevent deletion of TopologyDomain CRD when any Grove resource references it using
  finalizers

## Out of Scope

The following features are explicitly out of scope for this design:

- **Spread Constraints**: ReplicaSpreadDomain for distributing replicas across domains for fault tolerance is not
  supported
- **Advanced Topology Constraints Per Replica**: RootDomain for constraining entire resource (all replicas) within a
  topology domain is not supported
- **Ratio Grouping Between Groups**: AffinityGroups with PackRatio for complex workload patterns (e.g., 2 Prefill + 1
  Decode ratios) is not supported
- **Workload-Based Auto Constraints**: Automatic constraint generation based on workload characteristics, patterns, and
  inference requirements

## Architecture Context

### TopologyDomain CRD

Grove uses a TopologyDomain CRD to define the cluster's topology hierarchy dynamically. This allows flexible, extensible
topology definitions while maintaining immutability for stability.

#### CRD Structure

```go
// TopologyDomain defines the topology hierarchy for the cluster
// This resource is immutable after creation
// Only one TopologyDomain resource should exist per cluster (enforced by validation webhook)
type TopologyDomain struct {
metav1.TypeMeta   `json:",inline"`
metav1.ObjectMeta `json:"metadata,omitempty"`

Spec   TopologyDomainSpec   `json:"spec,omitempty"`
Status TopologyDomainStatus `json:"status,omitempty"`
}

type TopologyDomainSpec struct {
// Levels is an ordered list of topology levels from broadest to narrowest scope
// The order in this list defines the hierarchy (index 0 = highest level)
// This field is immutable
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="levels list is immutable"
// +kubebuilder:validation:MinItems=1
// +kubebuilder:validation:MaxItems=10
Levels []TopologyLevel `json:"levels"`
}

type TopologyLevel struct {
// Name is the level identifier used in TopologyConstraint references
// Must be a valid DNS label (lowercase alphanumeric with hyphens)
// Examples: "zone", "rack", "host"
// +kubebuilder:validation:Required
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
Name string `json:"name"`

// TopologyKey is the node label key that identifies this topology domain
// Must be a valid Kubernetes label key (qualified name)
// Examples: "topology.kubernetes.io/zone", "kubernetes.io/hostname"
// +kubebuilder:validation:Required
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=316
// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
TopologyKey string `json:"topologyKey"`

// Description provides human-readable information about this level
// +kubebuilder:validation:MaxLength=256
// +optional
Description string `json:"description,omitempty"`
}

type TopologyDomainStatus struct {
// ObservedGeneration reflects the generation most recently observed by the controller
// +optional
ObservedGeneration int64 `json:"observedGeneration,omitempty"`

// Conditions represent observations of the topology domain's current state
// Known condition types: Ready, KueueTopologyConsistent, InUse
// +optional
// +patchMergeKey=type
// +patchStrategy=merge
// +listType=map
// +listMapKey=type
Conditions []metav1.Condition `json:"conditions,omitempty"`

// UsedBy tracks Grove resources referencing this topology
// Helps administrators understand deletion impact
// +optional
UsedBy *TopologyUsage `json:"usedBy,omitempty"`
}

type TopologyUsage struct {
// PodCliqueSetCount is the number of PodCliqueSet resources using this topology
// Controller scans cluster on reconciliation to maintain this count
PodCliqueSetCount int32 `json:"podCliqueSetCount"`
}
```

#### Default TopologyDomain Resource (Provided in Grove Chart)

**Important:** Cluster admins MUST customize the `topologyKey` fields to match their actual cluster node labels before
applying. This resource is provided as a template.

```yaml
apiVersion: grove.run.ai/v1alpha1
kind: TopologyDomain
metadata:
  name: default
spec:
  levels:
    - name: region
      topologyKey: "topology.kubernetes.io/region"
      description: "Cloud provider region"
    - name: zone
      topologyKey: "topology.kubernetes.io/zone"
      description: "Availability zone within region"
    - name: datacenter
      topologyKey: "topology.kubernetes.io/datacenter"
      description: "Data center within zone"
    - name: block
      topologyKey: "topology.kubernetes.io/block"
      description: "Switching block within datacenter"
    - name: rack
      topologyKey: "topology.kubernetes.io/rack"
      description: "Network rack grouping"
    - name: host
      topologyKey: "kubernetes.io/hostname"
      description: "Individual compute host"
    - name: numa
      topologyKey: "topology.kubernetes.io/numa"
      description: "NUMA node within host"
```

#### Cluster Admin Customization Requirements

Before applying the TopologyDomain resource, cluster admins must customize it to match their specific cluster topology:

**Required Customizations:**

- **Update `topologyKey` fields**: Each `topologyKey` must match the actual node label keys used in your cluster
- The predefined level names and descriptions serve as a guide for proper configuration

**Optional Customizations:**

- **Modify `name` fields**: Change level names for clarity in your environment
- **Add topology levels**: Include additional levels if your topology requires them
- **Remove unused levels**: Delete levels that don't apply to your cluster

**Important Notes:**

- **Must apply TopologyDomain before creating any Grove workloads** (PodCliqueSet resources)
- **Once applied, the resource becomes immutable** - no modifications are allowed after creation
- Ensure node labels are correctly configured on cluster nodes before applying

**Domain Hierarchy**:

- List order defines hierarchy (index 0 = highest/broadest, last index = lowest/narrowest)
- Example: `region` (index 0) > `zone` (index 1) > `rack` (index 4) > `host` (index 5) > `numa` (index 6)

#### TopologyDomain Validation Rules

**CRD-Level Validations**:

- At least one level required in list (minimum 1, maximum 10)
- Level `name` is required, must be valid DNS label (1-63 chars, lowercase alphanumeric with hyphens)
- Level `topologyKey` is required, must be valid Kubernetes label key
- Level `description` is optional
- Entire levels list is immutable after creation

**Webhook Validations**:

- **Singleton Enforcement**: Only one TopologyDomain resource allowed per cluster
- **Unique Names**: Each level name in the list must be unique
- **Unique Keys**: Each topologyKey in the list must be unique
- **Immutability**: Cannot modify any field after creation
- **Deletion Prevention**: Managed by TopologyDomain controller using finalizer `grove.run.ai/topology-protection`.
  Controller checks for PodCliqueSet existence before allowing deletion (see TopologyDomain Controller section)

#### Kueue Topology Integration

Grove uses its own TopologyDomain CRD for defining topology levels and hierarchy, but must also integrate with Kueue
Topology
CRD which is required by KAI scheduler.

**Two CRDs Working Together:**

1. **TopologyDomain CRD** (Grove-specific):
    - Defines level names, topology keys, and hierarchy (via list order)
    - Used for validation and Grove API
    - Cluster admins create this for Grove resources
    - Immutable and singleton

2. **Kueue Topology CRD** (KAI requirement):
    - Required by KAI scheduler for actual scheduling
    - Must have matching topology keys in same order as TopologyDomain
    - Grove operator validates consistency between both CRDs

**Example Consistency:**

```yaml
# Grove TopologyDomain
apiVersion: grove.run.ai/v1alpha1
kind: TopologyDomain
metadata:
  name: default
spec:
  levels:
    - name: zone
      topologyKey: "topology.kubernetes.io/zone"
    - name: rack
      topologyKey: "topology.kubernetes.io/rack"
    - name: host
      topologyKey: "kubernetes.io/hostname"
```

```yaml
# Kueue Topology (must match topology keys in same order)
apiVersion: kueue.x-k8s.io/v1alpha1
kind: Topology
metadata:
  name: default
spec:
  levels:
    - nodeLabel: "topology.kubernetes.io/zone"
    - nodeLabel: "topology.kubernetes.io/rack"
    - nodeLabel: "kubernetes.io/hostname"
```

**Consistency Validation:**

- Grove validates that all topologyKeys in TopologyDomain.Spec.Levels exist in Kueue Topology in the same order
- Prevents scheduling failures due to mismatched topology definitions
- Admin workflow: Create TopologyDomain first, then create matching Kueue Topology

### TopologyDomain Controller

Grove includes a dedicated controller that manages TopologyDomain resource lifecycle. Currently, this controller handles
only deletion protection.

#### Deletion Protection Mechanism

The TopologyDomain controller implements deletion protection using a Kubernetes finalizer to prevent accidental deletion
of topology configuration while Grove workloads are running.

**Finalizer:**

- Controller adds finalizer `grove.run.ai/topology-protection` to the TopologyDomain resource
- Single finalizer protects against all Grove workloads
- Finalizer presence blocks deletion until removed by controller

#### Deletion Workflow

When an administrator attempts to delete the TopologyDomain resource:

1. **Deletion Request**: Admin runs `kubectl delete topologydomain default`
2. **Kubernetes Blocks**: Deletion is blocked due to finalizer presence
3. **Controller Reconciles**:
    - Controller detects deletion request (deletion timestamp is set)
    - Scans cluster for any PodCliqueSet resources
    - **If PodCliqueSet exists**: Controller keeps finalizer, deletion remains blocked
    - **If no PodCliqueSet exists**: Controller removes finalizer, deletion proceeds
4. **Deletion Completes**: Once finalizer is removed, Kubernetes deletes the TopologyDomain

**Why Only Check PodCliqueSet:**

- Due to Grove's ownership hierarchy, PodCliqueSet owns PodCliqueScalingGroup and PodClique
- If no PodCliqueSet exists, other resources (PCSG, PCLQ) cannot exist
- Simplified check improves controller performance

**Key Points:**

- Admin must delete all PodCliqueSet resources before deleting TopologyDomain
- Controller continuously reconciles to ensure protection
- Prevents orphaned workloads with invalid topology references

#### TopologyConstraint Model

```go
type TopologyConstraint struct {
// PackDomain references a level name from TopologyDomain.Spec.Levels
// Defines required topology packing constraint for replicas
// Replicas will be packed together within the specified topology level for network locality
// Default: admin-configured default level
PackDomain *string `json:"packDomain,omitempty"`
}
```

**Validation Rules**:

- Referenced level name must exist in TopologyDomain.Spec.Levels list
- Child resource PackDomain must be equal to or stricter (higher index) than parent resource PackDomain

## Component Architecture

### Non-Power User Flow (Automatic)

```
┌─────────────────┐    ┌──────────────────┐    ┌───────────────────┐
│ User Creates    │    │ Mutation Webhook │    │ Grove Operator    │
│ PodCliqueSet    │───▶│ Auto-populates   │───▶│ Applies Default   │
│ (no topology)   │    │ Admin Default    │    │ Pack Domain       │
└─────────────────┘    │ Topology         │    │ Constraint        │
                       └──────────────────┘    └─────────┬─────────┘
                                                         │
                                                         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                Grove Scheduler API (PodGang)                            │
│ • TopologyConstraint: Auto-generated pack domain constraint             │
│ • SpreadConstraints: Kubernetes-native spreading                        │
└─────────────────────────────────────────────────────────────────────────┘
```

### Power User Flow (Explicit)

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│ User Creates    │    │ Validation       │    │ Grove Operator      │
│ PodCliqueSet    │───▶│ Webhook Checks   │───▶│ Processes User      │
│ with Topology   │    │ Domain Hierarchy │    │ Pack Domain         │
│ Constraints     │    │ Rules            │    │ Constraints         │
└─────────────────┘    └──────────────────┘    └─────────┬───────────┘
                                                         │
                                                         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                Grove Scheduler API (PodGang)                            │
│ • TopologyConstraint: User-defined pack domain constraint               │
│ • SpreadConstraints: Kubernetes-native spreading                        │
└─────────────────────────────────────────────────────────────────────────┘
```

### Topology-Aware Scheduling Flow

The complete flow follows these high-level steps:

1. **Auto-Population**: Mutation webhook auto-populates admin topology defaults when user doesn't specify topology
   configuration
2. **Hierarchical Validation**: System validates domain existence and parent-child constraint relationships
3. **Pack Domain Translation**: Operator translates pack domain constraints from user API to scheduler API format:
    - PackDomain constraints are validated against TopologyDomain.Spec.Levels
    - Child resource constraints must be equal or stricter than parent constraints
4. **Scheduling**: Scheduler applies topology constraints with automatic fallback from preferred to less strict domains
5. **Finalizer Management**: Manage TopologyDomain CRD finalizers to prevent deletion when Grove resources reference
   them

#### Main Sequence Diagram

```
┌──────────────┐    ┌──────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ PodCliqueSet │    │  Grove Operator  │    │ Grove Scheduler │    │   Scheduler     │
│              │    │                  │    │      API        │    │                 │
└──────┬───────┘    └─────────┬────────┘    └────────┬────────┘    └────────┬────────┘
       │                      │                      │                      │
       │ CREATE/UPDATE        │                      │                      │
       ├─────────────────────▶│                      │                      │
       │                      │                      │                      │
       │                      │ 1. Mutation webhook  │                      │
       │                      │    auto-populates    │                      │
       │                      │    admin topology    │                      │
       │                      │                      │                      │
       │                      │ 2. Validate domain   │                      │
       │                      │    hierarchy rules   │                      │
       │                      │                      │                      │
       │                      │ 3. Manage finalizers │                      │
       │                      │    (see detailed     │                      │
       │                      │     diagram below)   │                      │
       │                      │                      │                      │
       │                      │ 4. Translate pack    │                      │
       │                      │    domain constraints│                      │
       │                      │                      │                      │
       │                      │ CREATE/UPDATE PodGang│                      │
       │                      ├─────────────────────▶│                      │
       │                      │                      │                      │
       │                      │                      │ SCHEDULE Pods        │
       │                      │                      ├─────────────────────▶│
       │                      │                      │                      │
       │                      │                      │                      │ 5. Apply topology
       │                      │                      │                      │    constraints with
       │                      │                      │                      │    automatic fallback
       │                      │                      │                      │    to less strict domains
       │                      │                      │                      │
```

## API Design

This design introduces a simplified topology API that replaces previous group-level configurations with per-resource
topology constraints:

- **Per-Resource Control**: Each Grove resource (PodCliqueSet, PodCliqueScalingGroup, PodClique) has its own
  TopologyConstraint field
- **Simplified Structure**: Single PackDomain field instead of complex group configurations
- **No Spread Constraints**: Removes unsupported ReplicaSpreadConstraints field from PodCliqueSetSpec
- **Singleton TopologyDomain**: Cluster-scoped CRD eliminates need for topology name references in each resource

### Fields Removed from Current API

The following existing fields are removed and replaced by the new TopologyConstraint model:

**From PodCliqueSetSpec:**

- `ReplicaSpreadConstraints []corev1.TopologySpreadConstraint` - Removed (spread constraints not supported)

**From PodCliqueSetTemplateSpec:**

- `SchedulingPolicyConfig *SchedulingPolicyConfig` - Removed (replaced by TopologyConstraint)

**Types Removed:**

- `SchedulingPolicyConfig` struct - Removed entirely
- `NetworkPackGroupConfig` struct - Removed entirely (replaced by per-resource TopologyConstraint)

### Grove Operator API Changes

#### PodCliqueSet CRD Extensions

```go
type PodCliqueSetSpec struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for the entire PodCliqueSet
// This field is immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

#### PodCliqueScalingGroup CRD Extensions

```go
type PodCliqueScalingGroupConfig struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for the PodCliqueScalingGroup
// Must be equal to or stricter than parent PodCliqueSet constraints
// This field is immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

#### PodClique CRD Extensions

```go
type PodCliqueTemplateSpec struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for the PodClique
// Must be equal to or stricter than parent resource constraints
// This field is immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

### Grove Scheduler API Changes

The Grove Operator translates user-defined topology configuration into Grove Scheduler API format.

#### Enhanced PodGang Structure

```go
type PodGangSpec struct {
// PodGroups is a list of member pod groups in the PodGang
PodGroups []PodGroup `json:"podgroups"`

// TopologyConstraint provides topology packing requirements for entire pod gang
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`

// PriorityClassName is the name of the PriorityClass for the PodGang
PriorityClassName string `json:"priorityClassName,omitempty"`
}
```

**Note:** SpreadConstraints field removed - spread constraints are not supported in this design.

## Topology Deletion Prevention

### Why Topology Deletion Prevention is Critical

Topology CRDs define the cluster's physical layout that Grove uses for optimal pod placement. Accidental deletion of a
topology CRD while PodGangSets reference it would cause:

- **Scheduling Failures**: New pods cannot be scheduled as the topology information is missing
- **Operational Confusion**: Administrators may not understand why scheduling suddenly fails

### Deletion Prevention Flow

Grove uses a stateless approach to prevent topology CRD deletion when PodGangSets reference them:

- **Finalizer Addition**: When any PodGangSet references a topology CRD, Grove adds a finalizer to the topology resource
- **Deletion Blocking**: When topology deletion is attempted, Grove scans all PodGangSets in the cluster to check for
  references, and blocks deletion
- **Cleanup Process**: Finalizers are removed when no PodGangSets reference the topology CRD (determined by cluster
  scan)

### Implementation Pattern

```yaml
# Topology with Grove finalizer
apiVersion: kueue.x-k8s.io/v1alpha1
kind: Topology
metadata:
  name: "default"
  finalizers:
    - "grove.ai/topology-protection"
```

### Topology Deletion Prevention Sequence Diagram

```
┌──────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Admin/User   │    │  Grove Operator  │    │ Topology CRD    │
│              │    │                  │    │                 │
└──────┬───────┘    └─────────┬────────┘    └─────────┬───────┘
       │                      │                        │
       │ DELETE topology      │                        │
       │ "default"            │                        │
       ├─────────────────────▶│                        │
       │                      │                        │
       │                      │ Deletion webhook       │
       │                      │ triggered              │
       │                      │                        │
       │                      │ Scan all PodGangSets   │
       │                      │ in cluster for refs    │
       │                      │ to "default"           │
       │                      │                        │
       │                      │ Found references?      │
       │                      │                        │
       │                      │ YES: Keep finalizer    │
       │                      │ Deletion blocked       │
       │                      │                        │
       │ Deletion blocked     │                        │
       │ (finalizer present)  │                        │
       │◀─────────────────────┤                        │
       │                      │                        │
       │                      │ NO: Remove finalizer   │
       │                      │                        │
       │                      ├───────────────────────▶│
       │                      │                        │
       │ Topology deleted     │                        │
       │◀─────────────────────┤                        │
       │                      │                        │
```

## Topology Configuration

### Admin Configuration

Administrators configure the default topology name in the operator deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grove-operator
spec:
  template:
    spec:
      containers:
        - name: operator
          args:
            - --default-topology-name=default-topology  # Kueue topology CRD name
```

## Validation Rules

### Validation Logic

The validation webhook ensures topology configuration consistency through the following checks:

### Domain Hierarchy Constraints

- **Level Existence**: Referenced PackDomain name must exist in TopologyDomain.Spec.Levels list
- **Parent-Child Validation**: Child resource PackDomain must be equal to or stricter than parent PackDomain
    - PodCliqueSet → PodCliqueScalingGroup → PodClique constraint hierarchy
    - Stricter means higher index (narrower scope) in TopologyDomain.Spec.Levels list
    - Example: If parent uses "zone" (index 1), child can use "zone", "rack" (index 4), or "host" (index 5)

### Immutability Constraints

- All `TopologyConstraint` fields are immutable after resource creation
- Domain hierarchy relationships cannot be changed after creation

## Mutation Webhook Rules

### Admin Default Auto-Population

The mutation webhook handles automatic topology configuration:

- Automatically populates default pack domain constraint from admin configuration when user hasn't specified any
- Applies admin-configured default PackDomain (e.g., "zone", "rack", or "host")
- Only applies to NEW Grove scalable resources (PodCliqueSet, PodCliqueScalingGroup, PodClique)
- Preserves user-specified topology constraints

### Admin Default Change Handling

When admin changes the default topology configuration in operator deployment:

- Existing Grove resources are NOT updated due to immutability
- Only NEW Grove resources receive the new defaults
- Prevents unexpected modifications to running workloads

