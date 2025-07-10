# Pod Discovery Design

**Status**: Draft  
**Date**: 2025-01-09  
**Authors**: Grove Maintainers  
**Scope**: MVP - Environment variable-based pod discovery

## Problem Statement

### Requirements: LeaderWorkerSet Parity with Multi-Leader Support

The goal is to provide the same pod discovery experience as LeaderWorkerSet (LWS) but with Grove's key advantage: **supporting multiple leader types within the same PodCliqueScalingGroup replica**.

#### Current LeaderWorkerSet Limitations
- **Single Leader Type**: LWS only supports one leader type per replica
- **Disaggregated Serving Constraint**: Cannot model prefill-leader + decode-leader in the same scaling unit
- **Role Inflexibility**: Fixed leader/worker pattern doesn't match modern inference architectures

#### Grove Requirements
1. **LWS Experience Parity**: Provide equivalent pod discovery capabilities to what users expect from LeaderWorkerSet
2. **Multi-Leader Support**: Enable multiple leader types (e.g., `PREFILL_LEADER`, `DECODE_LEADER`) in the same PCSG replica
3. **Disaggregated Serving**: Support modern inference patterns where different leader types coordinate within the same scaling unit
4. **Framework Compatibility**: Work with inference frameworks that expect leader-worker discovery patterns


## Current State

### Existing Architecture
Grove operator currently provides:
- **PodGangSet**: Top-level abstraction for inference serving systems with gang scheduling
- **PodCliqueScalingGroup**: Groups of PodCliques that scale together as super-pods (e.g., prefill group, decode group)
- **PodClique**: Individual workload units that create pods with identical specifications
- **Headless Services**: Service discovery at PodGangSet level only
- **Labeling System**: Comprehensive labeling for resource identification

### Limitations
- No way to distinguish roles within or across PodCliques
- No role-based service discovery within PodGangSets
- Limited environment variable injection capabilities
- No standardized way for pods to discover peers by role
- Manual configuration required for inter-pod communication
- Inference frameworks must implement their own role identification and discovery mechanisms

## Solution: Environment Variable-Based Discovery

### Example Scenario

For reference, consider this Grove setup:
- **PodGangSet**: `inf` (replicas=1) in namespace `dynamo`
- **PodCliqueScalingGroup**: `decode` (replicas=1)
- **PodCliques**: `dleader` (replicas=1), `dworker` (replicas=4)
- **Resulting Pods**: `inf-0-decode-0-dleader-xyzsd`, `inf-0-decode-0-dworker-ksjdd`, etc.

### Approach: Minimal Role-Based Discovery

**Implementation:**
- Add `roleName` field to `PodCliqueTemplateSpec` for role identification
- Provide minimal set of variables focused on role-based leader discovery
- Scope limited to single PCSG replica (scaling limitation acknowledged)

**Environment Variables Provided:**

*Role Information:*
```bash
ROLE_NAME=LEADER
ROLE_INDEX=0
```

*Role-Based Leader Discovery (within same PCSG replica):*
```bash
LEADER_ADDR=inf-0-decode-0-dleader-0.inf0.dynamo.svc.cluster.local
WORKER_ADDR=inf-0-decode-0-dworker-0.inf0.dynamo.svc.cluster.local
```

**Constraints and Limitations:**
- ✅ **Works within single PCSG replica**: All discovery within one replica works perfectly
- ❌ **Does not work across PCSG replicas**: Autoscaling creates independent replicas
- ❌ **Static at pod creation**: Environment variables cannot be updated during pod lifetime
- ✅ **Multi-leader support**: Can have multiple leader types in same replica (key differentiator vs LWS)

**Why This Approach:**
1. **LeaderWorkerSet Parity**: Provides equivalent experience within replica scope
2. **Multi-Leader Advantage**: Enables disaggregated serving patterns LWS cannot support
3. **Simple Implementation**: Minimal variables reduce complexity
4. **Framework Compatibility**: Matches inference framework expectations
5. **Clear Limitations**: Scaling constraints are explicit and understood

## Detailed Design

### API Changes

Add `roleName` field to `PodCliqueTemplateSpec`:

```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
spec:
  template:
    cliques:
      - name: dleader
        spec:
          roleName: LEADER          # NEW: Role identifier
          replicas: 1
          podSpec: # ... pod specification
      - name: dworker
        spec:
          roleName: WORKER          # NEW: Role identifier
          replicas: 4
          podSpec: # ... pod specification
```

### Environment Variable Injection (Proposal 2)

#### Core Role Variables
Each pod receives its role information:

```bash
ROLE_NAME=<role-name>        # e.g., LEADER, WORKER
ROLE_INDEX=<replica-index>   # e.g., 0, 1, 2
```

#### Leader Discovery Variables
Each pod receives DNS addresses for role leaders (index 0 of each role):

```bash
<ROLE_NAME>_ADDR=<leader-dns-address>
```

For the example scenario:
```bash
LEADER_ADDR=inf-0-decode-0-dleader-0.inf0.dynamo.svc.cluster.local
WORKER_ADDR=inf-0-decode-0-dworker-0.inf0.dynamo.svc.cluster.local
```

### Environment Variable Scoping Rules

#### For PodCliques in PodCliqueScalingGroups (PCSG)
- **Role Uniqueness**: Each role must be unique within the PCSG
- **Variable Injection**: All roles defined in the PCSG get entered as env vars into ALL pods belonging to that PCSG
- **Example**: If PCSG contains PodCliques with roles `PREFILL_LEADER`, `DECODE_LEADER`, `WORKER`:
  ```bash
  # Every pod in this PCSG gets ALL these variables:
  PREFILL_LEADER_ADDR=inf-0-decode-0-prefill-0.inf0.dynamo.svc.cluster.local
  DECODE_LEADER_ADDR=inf-0-decode-0-decode-0.inf0.dynamo.svc.cluster.local
  WORKER_ADDR=inf-0-decode-0-worker-0.inf0.dynamo.svc.cluster.local
  ```

#### For Standalone PodCliques (NOT part of any PCSG)
- **Role Uniqueness**: Each role must be unique among ALL standalone PodCliques in the PodGangSet
- **Variable Injection**: All standalone roles get entered as env vars into ALL standalone pods
- **Example**: If PodGangSet has standalone PodCliques with roles `API_GATEWAY`, `MONITOR`:
  ```bash
  # Every standalone pod gets ALL these variables:
  API_GATEWAY_ADDR=inf-0-gateway-0.inf0.dynamo.svc.cluster.local
  MONITOR_ADDR=inf-0-monitor-0.inf0.dynamo.svc.cluster.local
  ```

#### Cross-Scope Isolation
- **PCSG pods**: Only get env vars for roles within their PCSG (no standalone role vars)
- **Standalone pods**: Only get env vars for standalone roles (no PCSG role vars)
- **Different PCSGs**: Do not share env vars with each other

### Validation Requirements

#### RoleName Field Validation
- **Format**: Role names must be valid identifiers (alphanumeric, underscores, hyphens)
- **Case**: Typically uppercase for environment variables (e.g., `LEADER`, `WORKER`, `PREFILL_LEADER`)
- **Uniqueness**: Enforced within appropriate scope (PCSG or PodGangSet-wide for standalone)

#### Scope-Based Validation
- **Within PCSG**: Validate role uniqueness among all PodCliques within each PodCliqueScalingGroup
- **Standalone PodCliques**: Validate role uniqueness among ALL standalone PodCliques in the entire PodGangSet
- **Cross-Reference**: Ensure scaling group references point to valid PodCliques with roles
- **Scope Isolation**: Validate that role names don't conflict across scopes (PCSG vs standalone)

## Key Design Constraint: Role Uniqueness

### The Role Conflict Problem

Having the same role (like "LEADER") in the same PCSG creates fundamental conflicts:

1. **Environment Variable Conflicts**: Multiple PodCliques with `roleName: LEADER` would create conflicting `LEADER_ADDR` variables
2. **Ambiguous Discovery**: Frameworks wouldn't know which "LEADER" to connect to
3. **DNS Conflicts**: Multiple leaders would claim the same DNS name pattern

### Solution: Enforce Unique Roles Within PCSG

**Rule**: Each role must be unique within a PodCliqueScalingGroup

**Example**: Within same PCSG, use distinct roles:
- `PREFILL_LEADER` - for prefill coordination
- `DECODE_LEADER` - for decode coordination  
- `WORKER` - for inference processing

**Validation**: Admission webhook rejects PodGangSets with duplicate roles in same PCSG

## MVP Limitation: Single PCSG Replica Scope

### Environment Variable Constraint

**Fundamental Limitation**: Environment variables are static at pod creation and cannot be updated during pod lifetime.

**MVP Scope**: Environment variables work reliably only within a single PodCliqueScalingGroup replica.

#### What This Means

- **Within PCSG Replica**: ✅ Complete discovery works perfectly
- **Across PCSG Replicas**: ❌ No environment variable discovery
- **Autoscaling**: ❌ New replicas are independent units
- **Load Balancing**: External mechanism needed for cross-replica traffic

#### Why This Constraint is Acceptable for MVP

1. **Inference Patterns**: Frameworks establish connections at startup and maintain them
2. **Replica Independence**: Each PCSG replica is a complete inference unit
3. **LeaderWorkerSet Parity**: LWS has same limitation (single replica scope)
4. **Clear Expectations**: Users understand the scope and limitations

## MVP Focus: Multi-Leader Support in Single PCSG Replica

### LeaderWorkerSet Comparison

#### What LeaderWorkerSet Provides
```bash
# In LeaderWorkerSet, each pod gets:
LWS_LEADER_ADDRESS=leader-0.service.namespace.svc.cluster.local
REPLICA_ID=0
POD_NAME=workerset-0-leader-0
```

#### Grove MVP Equivalent (Proposal 2)
```bash
# In Grove PCSG replica, each pod gets:
ROLE_NAME=PREFILL_LEADER  # or DECODE_LEADER, WORKER
ROLE_INDEX=0
PREFILL_LEADER_ADDR=inf-0-decode-0-prefill-0.inf0.dynamo.svc.cluster.local
DECODE_LEADER_ADDR=inf-0-decode-0-decode-0.inf0.dynamo.svc.cluster.local
WORKER_ADDR=inf-0-decode-0-worker-0.inf0.dynamo.svc.cluster.local
```

### Key MVP Differentiator

**LeaderWorkerSet**: Single leader type per replica
```yaml
# LWS - Cannot do this in one replica
prefill-leader + decode-leader + workers  ❌
```

**Grove**: Multiple leader types per PCSG replica
```yaml
# Grove - CAN do this in one replica  
prefill-leader + decode-leader + workers  ✅
```

### MVP Scope Constraints

1. **Single PCSG Replica Focus**: Environment variables work within one PCSG replica only
2. **LWS Feature Parity**: Provide equivalent discovery capabilities to LeaderWorkerSet
3. **Multi-Leader Extension**: Add support for multiple leader types in same replica
4. **Simple Validation**: Enforce unique roles within PCSG replica scope

## Design Rationale

## Proposed API Changes

### Single Field Approach (Recommended)
Add a `roleName` field to `PodCliqueSpec` to identify the role of pods created by each PodClique.

**Pros of Single Field:**
- ✅ No breaking changes required
- ✅ Simple to implement and use
- ✅ Sufficient for MVP requirements
- ✅ Can encode both role and component info: `"prefill-leader"`, `"decode-worker"`

**Cons of Single Field:**
- ❌ Less structured than separate fields
- ❌ Requires parsing for role vs component distinction

### Alternative: Two Field Approach (Future Consideration)
If we needed to separate role and component in the future, we could add an optional `ComponentName` field with a clear migration strategy that maintains backward compatibility.

### Validation Requirements

#### Admission Webhook Validation
The following validation rules must be implemented in the PodGangSet admission webhook:

**RoleName Validation:**
- DNS-1123 compliant (already handled by existing validation)
- Role name length constraints for pod naming compatibility
- Role uniqueness within PodGangSet scope (optional business rule)

**Cross-Scaling Group Role Validation:**
- Validate that roles are consistent across scaling groups
- This ensures proper service discovery across PodGangSet scope

**Multi-Leader Validation within Same Scaling Group:**
- Validate multiple leader types within the same scaling group
- Ensure that multiple leaders have distinct role names

### Recommended MVP Approach
**Keep the single `RoleName` field** because:
1. **No Breaking Changes**: Critical for production deployments
2. **MVP Simplicity**: Single field handles most use cases
3. **Future Extensible**: Can add `ComponentName` later without breaking existing deployments
4. **Encoding Flexibility**: Users can encode component info in role name: `"prefill-leader"`, `"decode-coordinator"`

### Proposed API Structure

The pod discovery system extends the existing Grove API structure with minimal changes:

#### Current PodGangSet Structure
```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
spec:
  template:
    cliques:                              # Individual PodCliques
      - name: <clique-name>
        spec:
          replicas: <replica-count>
          podSpec: <pod-specification>
    podCliqueScalingGroups:              # Optional: Groups that scale together
      - name: <scaling-group-name>
        cliqueNames: [<clique-names>]    # References to cliques above
```

#### Proposed Enhancement
```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
spec:
  template:
    cliques:                              # Individual PodCliques with roles
      - name: <clique-name>
        spec:
          roleName: <role-identifier>     # NEW: Used for pod discovery
          replicas: <replica-count>
          podSpec: <pod-specification>
    podCliqueScalingGroups:              # Optional: Groups that scale together
      - name: <scaling-group-name>
        cliqueNames: [<clique-names>]    # References to cliques above
```

#### Key Design Decisions
- **`roleName`**: New field in `PodCliqueSpec` that identifies the role of pods
- **`cliques`**: Existing list of PodClique templates, now with distinct roles
- **`podCliqueScalingGroups`**: Existing groupings for scaling together (super-pods), now role-aware

## Design Rationale

### Why Environment Variables?
Environment variables provide the simplest and most universally supported mechanism for service discovery in containerized environments:
- **Framework Agnostic**: Works with any inference framework (TensorRT-LLM, SGLang, vLLM, etc.)
- **Container Native**: Standard mechanism supported by all container runtimes
- **Zero Dependencies**: No additional service discovery infrastructure required
- **Immediate Availability**: Available at pod startup without waiting for external services

### Why Single `roleName` Field?
The design chooses a single field approach over more complex alternatives:
- **Simplicity**: Single field is easier to understand and use
- **Flexibility**: Can encode component info in role name (e.g., `"prefill-leader"`, `"decode-worker"`)
- **Non-Breaking**: Additive change that doesn't affect existing PodGangSets
- **Future Proof**: Can extend with additional fields later if needed

### Why Role-Based Over Name-Based Discovery?
Using roles instead of PodClique names provides better abstraction:
- **Semantic Clarity**: Roles describe function, names describe implementation
- **Framework Alignment**: Inference frameworks think in terms of roles (leader/worker)
- **Deployment Flexibility**: Same role can have different PodClique names across environments
- **Multi-Leader Support**: Multiple PodCliques can have leader roles in same scaling group

### Architecture Benefits
This design leverages Grove's existing architecture strengths:
- **PodGangSet Scope**: Discovery works across entire inference system
- **Scaling Group Awareness**: Understands relationships between PodCliques
- **Gang Scheduling**: All discovered pods are gang scheduled together
- **Topology Awareness**: Integrates with Grove's network placement optimizations


## Example Configurations

### Example 1: Simple Leader-Worker Pattern
Basic inference setup with single leader and multiple workers:

```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
metadata:
  name: simple-inference
  namespace: inference-system
spec:
  replicas: 1
  template:
    cliques:
      - name: leader-clique
        spec:
          roleName: leader
          replicas: 1
          podSpec:
            containers:
              - name: leader
                image: my-inference-app:latest
                ports:
                  - containerPort: 8080
                env:
                  - name: ROLE_TYPE
                    value: "leader"
      - name: worker-clique
        spec:
          roleName: worker  
          replicas: 3
          podSpec:
            containers:
              - name: worker
                image: my-inference-app:latest
                ports:
                  - containerPort: 8081
                env:
                  - name: ROLE_TYPE
                    value: "worker"
```

**Pod Discovery Environment Variables Generated:**
- Leader pod: `GROVE_ROLE=leader`, `GROVE_IS_LEADER=true`, `GROVE_ROLE_WORKER_COUNT=3`
- Worker pods: `GROVE_ROLE=worker`, `GROVE_IS_LEADER=false`, `GROVE_ROLE_LEADER_COUNT=1`

### Example 2: Multi-Leader with Scaling Groups
Disaggregated inference with multiple leader types in the same scaling group:

```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
metadata:
  name: llm-inference
  namespace: inference-system
spec:
  replicas: 1
  template:
    cliques:
      - name: prefill-leaders
        spec:
          roleName: prefill-leader
          replicas: 2
          podSpec:
            containers:
              - name: prefill-leader
                image: llm-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
                env:
                  - name: COMPONENT_TYPE
                    value: "prefill"
                  - name: ROLE_TYPE
                    value: "leader"
      - name: decode-leaders  
        spec:
          roleName: decode-leader
          replicas: 2
          podSpec:
            containers:
              - name: decode-leader
                image: llm-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
                env:
                  - name: COMPONENT_TYPE
                    value: "decode"
                  - name: ROLE_TYPE
                    value: "leader"
      - name: workers
        spec:
          roleName: worker
          replicas: 4
          podSpec:
            containers:
              - name: worker
                image: llm-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
                env:
                  - name: ROLE_TYPE
                    value: "worker"
    podCliqueScalingGroups:
      - name: inference-group
        cliqueNames: ["prefill-leaders", "decode-leaders", "workers"]
        scaleConfig:
          maxReplicas: 10
```

**Pod Discovery Environment Variables Generated:**
- Prefill leader: `GROVE_ROLE=prefill-leader`, `GROVE_ROLE_DECODE_LEADER_COUNT=2`, `GROVE_ROLE_WORKER_COUNT=4`
- Decode leader: `GROVE_ROLE=decode-leader`, `GROVE_ROLE_PREFILL_LEADER_COUNT=2`, `GROVE_ROLE_WORKER_COUNT=4`
- Worker: `GROVE_ROLE=worker`, `GROVE_ROLE_PREFILL_LEADER_COUNT=2`, `GROVE_ROLE_DECODE_LEADER_COUNT=2`

### Example 3: Mixed Scaling Groups + Standalone Components
Complex setup with both scaling groups and standalone PodCliques:

```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
metadata:
  name: multi-model-inference
  namespace: inference-system
spec:
  replicas: 1
  template:
    cliques:
      - name: text-leaders
        spec:
          roleName: text-leader
          replicas: 2
          podSpec:
            containers:
              - name: text-leader
                image: text-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
      - name: vision-leaders
        spec:
          roleName: vision-leader
          replicas: 2
          podSpec:
            containers:
              - name: vision-leader
                image: vision-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
      - name: inference-workers
        spec:
          roleName: worker
          replicas: 8
          podSpec:
            containers:
              - name: worker
                image: multi-modal-inference:latest
                resources:
                  limits:
                    nvidia.com/gpu: 1
      - name: standalone-gateway
        spec:
          roleName: api-gateway
          replicas: 1
          podSpec:
            containers:
              - name: gateway
                image: api-gateway:latest
                ports:
                  - containerPort: 80
                resources:
                  limits:
                    cpu: "2"
                    memory: "4Gi"
    podCliqueScalingGroups:
      - name: inference-group
        cliqueNames: ["text-leaders", "vision-leaders", "inference-workers"]
        scaleConfig:
          maxReplicas: 15
    # standalone-gateway remains outside scaling groups
```

**Pod Discovery Environment Variables Generated:**
- Text leader: `GROVE_ROLE=text-leader`, discovers other roles via `GROVE_ROLE_*_COUNT` variables
- Vision leader: `GROVE_ROLE=vision-leader`, discovers other roles via `GROVE_ROLE_*_COUNT` variables
- Worker: `GROVE_ROLE=worker`, discovers all leader types and standalone components
- API Gateway: `GROVE_ROLE=api-gateway`, discovers all roles across the entire PodGangSet

## Backward Compatibility

### Migration Strategy
1. **Gradual Adoption**: `RoleName` field is optional, defaults to PodClique name
2. **Environment Variable Opt-in**: Only inject discovery variables when `RoleName` is explicitly set
3. **Existing Deployments**: Continue to work without modification
4. **Service Discovery**: Maintain current headless service patterns

### Compatibility Guarantees
- Existing PodGangSet configurations remain functional
- Current labeling and service discovery patterns preserved
- No breaking changes to APIs or behavior
- Optional feature activation through configuration

## Conclusion

This pod discovery design provides a simple, extensible foundation for role-based service discovery in Grove operator that addresses key requirements for inference workloads:

### Key Design Decisions Summary

1. **Environment Variable Approach**: Chosen for universal framework compatibility and zero-dependency operation
2. **Single `roleName` Field**: Provides simplicity while maintaining flexibility for future extensions
3. **PodGangSet Scope Discovery**: Enables cross-scaling-group and standalone PodClique discovery
4. **Non-Breaking Addition**: Preserves all existing functionality while adding new capabilities

### Benefits for Inference Workloads

- **Multi-Leader Support**: Enables complex patterns like disaggregated serving with prefill/decode leaders
- **Framework Agnostic**: Works with TensorRT-LLM, SGLang, vLLM, and other inference frameworks
- **Operational Simplicity**: Reduces manual configuration burden for inference deployments
- **Grove Integration**: Leverages existing gang scheduling and topology awareness features

### Future Extensibility

The design establishes a foundation that can evolve with Grove's inference capabilities:
- Additional discovery mechanisms (ConfigMap, DNS-based)
- Enhanced role abstractions (component separation)
- Integration with observability and monitoring systems
- Support for advanced inference patterns as they emerge

This approach balances immediate MVP needs with long-term architectural flexibility, ensuring Grove can continue to lead in multinode inference orchestration.

## Future Considerations

The design establishes a foundation that can evolve with Grove's inference capabilities:
- Additional discovery mechanisms as needs become clearer
- Enhanced role abstractions if component separation becomes necessary
- Integration with monitoring and observability systems
- Support for advanced inference patterns as they emerge

## Conclusion

This pod discovery design provides a simple, extensible foundation for role-based service discovery in Grove operator. By leveraging environment variables and building on existing architecture, we can deliver immediate value while maintaining backward compatibility and enabling future enhancements.

The design supports complex scenarios like multi-leader configurations while keeping the implementation simple and Kubernetes-native. This approach aligns with Grove's goals of providing flexible, scalable infrastructure for AI/ML workloads.