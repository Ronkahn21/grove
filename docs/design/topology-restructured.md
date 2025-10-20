# Topology-Aware Scheduling - Grove Operator Design

## Overview

This document defines the design for implementing topology-aware scheduling in the Grove operator.

**Motivation**: Topology-aware scheduling is critical for Grove's multinode inference workloads because these
applications require:

- **Network Locality**: High-bandwidth communication between prefill and decode workers benefits from proximity
- **Coordinated Placement**: Related components (e.g., model shards) perform better when co-located within the same
  topology domain
- **Latency Optimization**: Minimizing network hops between interdependent inference components improves end-to-end
  performance

## Goals

- Provide flexible, cluster-agnostic topology hierarchy definition via TopologyDomain CRD
- Enable packing constraints for network locality across all Grove scalable resources
- Support multiple topology configurations for different environments
- Automatic Kueue Topology generation for KAI scheduler integration
- Immutable topology configuration ensuring scheduling consistency
- Hierarchical constraint validation (child stricter than parent)

## Non-Goals

- Spread constraints across topology domains (ReplicaSpreadDomain)
- Root domain constraints for entire resource (RootDomain)
- Ratio-based affinity groups between scaling groups (AffinityGroups with PackRatio)
- Dynamic topology reconfiguration after creation
- Per-workload topology domain selection
- Automatic topology inference from workload characteristics

## Proposal

### High-Level Approach

Grove implements topology-aware scheduling through three key components:

1. **TopologyDomain CRD**: Cluster-scoped resource defining topology hierarchy
    - Admin creates TopologyDomain with ordered list of topology levels
    - Each level maps friendly name (e.g., "rack") to node label key (e.g., "topology.kubernetes.io/rack")
    - Multiple TopologyDomains supported for different environments

2. **Operator Configuration**: References TopologyDomain by name
    - Operator argument `--topology-domain-name=default` selects which TopologyDomain to use
    - All workload validation performed against configured TopologyDomain
    - Enables switching between topologies without changing workloads

3. **Workload API (TopologyConstraint)**: Users specify packing requirements
    - PodCliqueSet, PodCliqueScalingGroup, and PodClique each have TopologyConstraint field
    - Users reference level names from TopologyDomain (e.g., `packDomain: "rack"`)
    - No direct TopologyDomain reference needed in workloads

### Component Interactions

```
TopologyDomain CRD ──┐
  (admin creates)    │
                     │
Operator Config ─────┼──> Operator validates PackDomain
  (--topology-       │    against TopologyDomain.Spec.Levels
   domain-name)      │
                     │
PodCliqueSet ────────┘
  (packDomain: "rack")
```

### Controller Responsibilities

The TopologyDomain controller manages:

- **Kueue Topology Generation**: Auto-creates Kueue Topology CRD for KAI scheduler integration
- **Deletion Protection**: Prevents deletion while PodCliqueSet resources reference it

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

## Design Details

### 1. TopologyDomain Infrastructure

#### TopologyDomain CRD

TopologyDomain is a cluster-scoped CRD that defines the topology hierarchy for scheduling. It maps friendly level names
to Kubernetes node labels and establishes ordering from broadest to narrowest scope.

**Characteristics:**

- **Cluster-scoped**: Multiple TopologyDomains can exist
- **Operator-selected**: Operator references one by name via `--topology-domain-name` argument
- **Immutable**: Once created, cannot be modified
- **List-ordered hierarchy**: Index 0 = broadest (e.g., region), last = narrowest (e.g., host)

**API Structure:**

```go
// TopologyDomain defines the topology hierarchy for the cluster
// This resource is immutable after creation
// Multiple TopologyDomain resources can exist; Grove operator references one via --topology-domain-name argument
type TopologyDomain struct {
metav1.TypeMeta   `json:",inline"`
metav1.ObjectMeta `json:"metadata,omitempty"`

Spec TopologyDomainSpec `json:"spec,omitempty"`
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
```

**Example TopologyDomain:**

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

**Creating TopologyDomain:**

Steps:

1. Install Grove: `helm install grove`
2. Customize example above with your cluster's actual `topologyKey` values
3. Create resource: `kubectl apply -f topologydomain.yaml`
4. Configure operator with `--topology-domain-name` matching the resource name
5. Create workloads with topology constraints

Notes:

- TopologyDomain becomes immutable after creation
- Multiple TopologyDomains can exist; operator uses the one specified in its argument
- Ensure node labels exist on cluster nodes before creating workloads
- List order defines hierarchy: index 0 = broadest, last = narrowest
- Example hierarchy: `region` (0) > `zone` (1) > `datacenter` (2) > `block` (3) > `rack` (4) > `host` (5) > `numa` (6)

**Validation:**

CRD-Level:

- At least one level required (minimum 1, maximum 10)
- Level `name` required, must be valid DNS label (1-63 chars)
- Level `topologyKey` required, must be valid Kubernetes label key (1-316 chars)
- Level `description` optional (max 256 chars)
- Entire levels list immutable after creation

Webhook:

- Each level name must be unique within TopologyDomain
- Each topologyKey must be unique within TopologyDomain
- Cannot modify any field after creation
- Deletion protection via controller finalizer

#### TopologyDomain Controller

The TopologyDomain controller manages the TopologyDomain resource lifecycle with two primary responsibilities:

**1. Kueue Topology Generation**

Automatically generates Kueue Topology CRD from the TopologyDomain referenced by operator's `--topology-domain-name`
argument.

Generation Process:

1. Controller watches TopologyDomain specified in operator argument
2. When TopologyDomain created, controller creates matching Kueue Topology
3. Kueue Topology name matches TopologyDomain name
4. Levels extracted from TopologyDomain.Spec.Levels using topologyKey field
5. Order preserved from TopologyDomain list

Example:

From TopologyDomain `default` with levels zone/rack/host, controller generates:

```yaml
apiVersion: kueue.x-k8s.io/v1alpha1
kind: Topology
metadata:
  name: default
  ownerReferences:
    - apiVersion: grove.run.ai/v1alpha1
      kind: TopologyDomain
      name: default
      controller: true
spec:
  levels:
    - nodeLabel: "topology.kubernetes.io/zone"
    - nodeLabel: "topology.kubernetes.io/rack"
    - nodeLabel: "kubernetes.io/hostname"
```

Key Points:

- Admin only creates TopologyDomain; Kueue Topology auto-generated
- Owner reference ensures Kueue Topology deleted with TopologyDomain
- Same name for both resources
- No manual coordination required

**2. Deletion Protection**

Prevents TopologyDomain deletion while PodCliqueSet resources reference it using Kubernetes finalizer.

Deletion Workflow:

1. Admin runs `kubectl delete topologydomain default`
2. Kubernetes blocks deletion (finalizer `grove.run.ai/topology-protection` present)
3. Controller reconciles:
    - Detects deletion request (deletion timestamp set)
    - Scans cluster for any PodCliqueSet resources
    - If PodCliqueSet exists: Keeps finalizer, deletion blocked
    - If no PodCliqueSet exists: Removes finalizer, deletion proceeds
4. Once finalizer removed, Kubernetes deletes TopologyDomain

Why Only Check PodCliqueSet:

- Grove ownership hierarchy: PodCliqueSet owns PodCliqueScalingGroup and PodClique
- If no PodCliqueSet exists, other resources cannot exist
- Simplified check improves performance

Key Points:

- Admin must delete all PodCliqueSet before deleting TopologyDomain
- Controller continuously reconciles
- Prevents orphaned workloads with invalid topology references

#### Operator Configuration

Operator references TopologyDomain by name via command-line argument:

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
            - --topology-domain-name=default  # References TopologyDomain by name
```

Configuration:

- `--topology-domain-name`: Specifies TopologyDomain resource name for validation
- Operator loads referenced TopologyDomain at startup
- All PodCliqueSet topology constraints validated against this TopologyDomain
- If TopologyDomain doesn't exist, operator logs error and disables topology features

Multiple Topologies:

- Multiple TopologyDomain resources can exist (e.g., "aws-topology", "on-prem-topology")
- Operator argument selects which one to use
- Enables different topology configurations per environment

### 2. Operator API Changes (Grove CRDs)

#### TopologyConstraint Model

```go
type TopologyConstraint struct {
// PackDomain references a level name from TopologyDomain.Spec.Levels
// Defines required topology packing constraint for replicas
// Replicas packed together within specified topology level for network locality
PackDomain *string `json:"packDomain,omitempty"`
}
```

#### Fields Removed from Current API

**From PodCliqueSetSpec:**

- `ReplicaSpreadConstraints []corev1.TopologySpreadConstraint` - Removed (spread not supported)

**From PodCliqueSetTemplateSpec:**

- `SchedulingPolicyConfig *SchedulingPolicyConfig` - Removed (replaced by TopologyConstraint)

**Types Removed:**

- `SchedulingPolicyConfig` struct - Removed entirely
- `NetworkPackGroupConfig` struct - Removed entirely

#### PodCliqueSet CRD Extensions

```go
type PodCliqueSetTemplateSpec struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for PodCliqueSet
// Immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
// +optional
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

#### PodCliqueScalingGroup CRD Extensions

```go
type PodCliqueScalingGroupConfig struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for PodCliqueScalingGroup
// Must be equal to or stricter than parent PodCliqueSet constraints
// Immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
// +optional
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

#### PodClique CRD Extensions

```go
type PodCliqueTemplateSpec struct {
// ... existing fields ...

// TopologyConstraint defines topology placement requirements for PodClique
// Must be equal to or stricter than parent resource constraints
// Immutable after resource creation
// +kubebuilder:validation:XValidation:rule="self==oldSelf",message="topology constraints are immutable"
// +optional
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`
}
```

#### Validation Webhook

The validation webhook ensures topology configuration consistency:

**TopologyDomain Reference:**

- TopologyDomain specified in operator's `--topology-domain-name` must exist
- Referenced PackDomain name must exist in TopologyDomain.Spec.Levels
- All validation performed against operator-configured TopologyDomain

**Hierarchy Constraints:**

- Child resource PackDomain must be equal to or stricter than parent
- PodCliqueSet → PodCliqueScalingGroup → PodClique hierarchy
- Stricter = higher index (narrower scope) in TopologyDomain.Spec.Levels
- Example: If parent uses "zone" (index 1), child can use "zone", "rack" (index 4), or "host" (index 5)

**Immutability:**

- All TopologyConstraint fields immutable after resource creation
- Domain hierarchy relationships cannot change after creation

#### Mutation Webhook

**Auto-Population:**

- Automatically populates default pack domain from admin configuration when not specified
- Applies admin-configured default PackDomain (e.g., "zone", "rack", "host")
- Only applies to NEW resources (PodCliqueSet, PodCliqueScalingGroup, PodClique)
- Preserves user-specified topology constraints

**Default Changes:**

- Existing resources NOT updated when admin changes defaults (immutability)
- Only NEW resources receive new defaults
- Prevents unexpected modifications to running workloads

### 3. Scheduler API Changes (Contract with KAI)

#### PodGang CRD Extensions

The Grove Operator translates topology configuration into Grove Scheduler API format, which serves as the contract with
KAI scheduler.

```go
type PodGangSpec struct {
// PodGroups is a list of member pod groups in the PodGang
PodGroups []PodGroup `json:"podgroups"`

// TopologyConstraint provides topology packing requirements for entire pod gang
// Translated from PodCliqueSet topology constraints
TopologyConstraint *TopologyConstraint `json:"topologyConstraint,omitempty"`

// PriorityClassName is the name of the PriorityClass for the PodGang
PriorityClassName string `json:"priorityClassName,omitempty"`
}
```

**Changes:**

- Added: `TopologyConstraint` field for packing constraints
- Removed: `SpreadConstraints` field (spread not supported)

**Translation Logic:**

- Operator translates PodCliqueSet.TopologyConstraint to PodGang.TopologyConstraint
- Validates PackDomain against operator-configured TopologyDomain
- KAI scheduler consumes TopologyConstraint for actual scheduling

## Component Architecture

### Topology-Aware Scheduling Flow

Complete end-to-end flow:

1. **Admin Setup**:
    - Create TopologyDomain with topology levels
    - Configure operator with `--topology-domain-name=default`
    - TopologyDomain controller creates Kueue Topology automatically

2. **User Creates Workload**:
    - User creates PodCliqueSet with `topologyConstraint.packDomain: "rack"`
    - Mutation webhook auto-populates if not specified

3. **Validation**:
    - Validation webhook checks PackDomain exists in TopologyDomain.Spec.Levels
    - Validates hierarchy rules (child stricter than parent)

4. **Translation**:
    - Operator translates TopologyConstraint to PodGang spec
    - Creates/updates PodGang in Grove Scheduler API

5. **Scheduling**:
    - KAI scheduler reads Kueue Topology and PodGang.TopologyConstraint
    - Applies packing constraints with automatic fallback to less strict levels

### Sequence Diagram

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
       │                      │                      │                      │
       │                      │ 2. Validation webhook│                      │
       │                      │    validates against │                      │
       │                      │    TopologyDomain    │                      │
       │                      │                      │                      │
       │                      │ 3. Translate to      │                      │
       │                      │    PodGang spec      │                      │
       │                      │                      │                      │
       │                      │ CREATE/UPDATE PodGang│                      │
       │                      ├─────────────────────▶│                      │
       │                      │                      │                      │
       │                      │                      │ SCHEDULE Pods        │
       │                      │                      ├─────────────────────▶│
       │                      │                      │                      │
       │                      │                      │                      │ Apply topology
       │                      │                      │                      │ using Kueue
       │                      │                      │                      │ Topology CRD
       │                      │                      │                      │
```

## Implementation Notes

### Edge Cases

**Case 1: PodCliqueSet Created Before TopologyDomain**

- If TopologyConstraint not set: Creation allowed (non-topology workload)
- If TopologyConstraint.PackDomain set: Creation rejected (cannot validate without TopologyDomain)

**Case 2: TopologyDomain Created After PodCliqueSet**

- Existing workloads not affected (continue without topology)
- New workloads created after TopologyDomain can use topology constraints
- Topology constraints immutable at workload creation time

**Design Principle:** Topology constraints established at creation remain immutable. Adding/removing TopologyDomain does
not retroactively affect existing workloads.

### Open Questions

- Need to define rollout changes to topology levels (immutable, so not supported)
- If spread-domain count < replica count, should extra replicas remain pending or schedule anyway?
- How will cluster admin map Grove topology constants to physical topology labels? (Answered: via TopologyDomain CRD)
- Should we allow changes to cluster topology levels and mappings? (No: immutable)
- Default kube-scheduler only supports TSC at pod level; how will domain-level packing be realized in KAI scheduler? (
  Discuss with scheduler team)
