# Pod Discovery Design

**Status**: Draft  
**Date**: 2025-01-09  
**Authors**: Grove Maintainers  

## Problem Statement

### Requirements for Inference Workloads

Modern inference frameworks (TensorRT-LLM, SGLang, vLLM) require pods within a Grove PodGangSet to discover and communicate with each other based on their roles rather than arbitrary names. Key requirements include:

1. **Role-Based Discovery**: Pods need to identify peers by functional role (e.g., "leader", "worker", "prefill-leader", "decode-worker")
2. **Multi-Leader Support**: Support for multiple leader types within the same PodCliqueScalingGroup for disaggregated serving patterns
3. **Framework Agnostic**: Work with any inference framework without requiring framework-specific integrations
4. **PodGangSet Scope**: Discovery must work across all PodCliques within a PodGangSet, both in scaling groups and standalone
5. **Startup Coordination**: Enable frameworks to coordinate initialization and establish communication patterns

### Why Environment Variables Are Essential

Environment variables are the universal standard for service discovery in containerized inference workloads because:

- **Framework Independence**: All inference frameworks can read environment variables without additional dependencies
- **Container Native**: Available immediately at pod startup without external service calls
- **Industry Standard**: Matches patterns used by Kubernetes StatefulSets, Jobs, and other orchestration systems
- **Simplicity**: No additional infrastructure, agents, or service discovery mechanisms required
- **Reliability**: Always available, even during network partitions or service discovery service outages

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

## Design Approaches

Based on analysis of inference framework requirements and Grove's architecture, we have two concrete proposals for pod discovery:

### Example Scenario

For reference, consider this Grove setup:
- **PodGangSet**: `inf` (replicas=1) in namespace `dynamo`
- **PodCliqueScalingGroup**: `decode` (replicas=1)
- **PodCliques**: `dleader` (replicas=1), `dworker` (replicas=4)
- **Resulting Pods**: `inf-0-decode-0-dleader-xyzsd`, `inf-0-decode-0-dworker-ksjdd`, etc.

### Proposal 1: Comprehensive Environment Variable Set

**Implementation:**
- Add `roleName` field to `PodCliqueTemplateSpec` for role identification
- Inject comprehensive set of environment variables for discovery and addressing
- Provide both individual pod discovery and role-based grouping

**Environment Variables Provided:**

*Basic Grove Information:*
```bash
GROVE_PGS_NAME=inf
GROVE_PGS_INDEX=0
GROVE_PCLQ_NAME=inf-0-decode-0-dleader
GROVE_PCLQ_INDEX=0
GROVE_HEADLESS_SERVICE=inf0.dynamo.svc.cluster.local
GROVE_HOSTNAME=inf-0-decode-0-dleader-0
```

*Role Information:*
```bash
ROLE_NAME=LEADER
ROLE_INDEX=0
GROVE_ROLES=LEADER,WORKER  # For PCSG members
```

*Individual Pod Discovery (for PCSG members):*
```bash
LEADER_0=inf-0-decode-0-dleader-0
WORKER_0=inf-0-decode-0-dworker-0
WORKER_1=inf-0-decode-0-dworker-1
WORKER_2=inf-0-decode-0-dworker-2
WORKER_3=inf-0-decode-0-dworker-3
```

*Full Address Discovery (for PCSG members):*
```bash
LEADER_0_ADDR=inf-0-decode-0-dleader-0.inf0.dynamo.svc.cluster.local
WORKER_0_ADDR=inf-0-decode-0-dworker-0.inf0.dynamo.svc.cluster.local
WORKER_1_ADDR=inf-0-decode-0-dworker-1.inf0.dynamo.svc.cluster.local
WORKER_2_ADDR=inf-0-decode-0-dworker-2.inf0.dynamo.svc.cluster.local
WORKER_3_ADDR=inf-0-decode-0-dworker-3.inf0.dynamo.svc.cluster.local
```

**Pros:**
- ✅ **Complete Information**: Provides all possible discovery data
- ✅ **Individual Pod Access**: Can address specific pod replicas by index
- ✅ **Framework Flexibility**: Supports diverse framework communication patterns
- ✅ **Debugging Friendly**: Rich information for troubleshooting

**Cons:**
- ❌ **Environment Variable Proliferation**: Large number of variables for big scaling groups
- ❌ **Memory Overhead**: Significant memory usage for environment variables
- ❌ **Complexity**: More variables to manage and understand

### Proposal 2: Minimal Role-Based Discovery

**Implementation:**
- Add `roleName` field to `PodCliqueTemplateSpec` for role identification
- Provide minimal set of variables focused on role-based leader discovery
- Emphasize DNS-based discovery patterns

**Environment Variables Provided:**

*Role Information:*
```bash
ROLE_NAME=LEADER
ROLE_INDEX=0
```

*Role-Based Leader Discovery:*
```bash
LEADER_ADDR=inf-0-decode-0-dleader-0.inf0.dynamo.svc.cluster.local
WORKER_ADDR=inf-0-decode-0-dworker-0.inf0.dynamo.svc.cluster.local
```

**Role Uniqueness Rules:**
- If PodClique is part of PCSG: Role must be unique within that PCSG
- If PodClique is standalone: Role must be unique among all standalone PodCliques
- Role variables injected based on scope (PCSG vs PodGangSet-wide)

**Pros:**
- ✅ **Simplicity**: Minimal set of variables to understand and manage
- ✅ **DNS-Native**: Leverages Kubernetes DNS for service discovery
- ✅ **Scalable**: Number of variables doesn't grow with replica count
- ✅ **Leader-Worker Pattern**: Optimized for common inference patterns

**Cons:**
- ❌ **Limited Individual Access**: Cannot directly address non-leader pod replicas
- ❌ **DNS Dependency**: Requires working cluster DNS for full functionality
- ❌ **Framework Constraints**: May not suit all communication patterns

## Recommended Approach: Proposal 2 (Minimal Role-Based)

We recommend **Proposal 2** because it balances functionality with simplicity:

### Why Proposal 2 for Inference Workloads

1. **Leader-Worker Dominance**: Most inference frameworks use leader-worker patterns where workers connect to leaders
2. **DNS Integration**: Kubernetes DNS provides robust service discovery that complements environment variables
3. **Scalability**: Environment variable count remains manageable even with large replica counts
4. **Operational Simplicity**: Fewer variables reduce debugging complexity and memory overhead
5. **Framework Alignment**: Matches patterns used by TensorRT-LLM, vLLM, and SGLang

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

### Role Scoping Rules

#### For PodCliques in PodCliqueScalingGroups
- **Role Uniqueness**: Each role must be unique within the PCSG
- **Variable Scope**: Role discovery variables include all roles within the same PCSG
- **Example**: If PCSG contains `PREFILL_LEADER`, `DECODE_LEADER`, `WORKER` roles:
  ```bash
  PREFILL_LEADER_ADDR=inf-0-decode-0-prefill-0.inf0.dynamo.svc.cluster.local
  DECODE_LEADER_ADDR=inf-0-decode-0-decode-0.inf0.dynamo.svc.cluster.local
  WORKER_ADDR=inf-0-decode-0-worker-0.inf0.dynamo.svc.cluster.local
  ```

#### For Standalone PodCliques
- **Role Uniqueness**: Each role must be unique among all standalone PodCliques in the PodGangSet
- **Variable Scope**: Role discovery variables include all standalone roles in the PodGangSet
- **Example**: If PodGangSet has standalone roles `API_GATEWAY`, `MONITOR`:
  ```bash
  API_GATEWAY_ADDR=inf-0-gateway-0.inf0.dynamo.svc.cluster.local
  MONITOR_ADDR=inf-0-monitor-0.inf0.dynamo.svc.cluster.local
  ```

### Validation Requirements

#### RoleName Field Validation
- **Format**: Role names must be valid identifiers (alphanumeric, underscores, hyphens)
- **Case**: Typically uppercase for environment variables (e.g., `LEADER`, `WORKER`, `PREFILL_LEADER`)
- **Uniqueness**: Enforced within appropriate scope (PCSG or PodGangSet-wide for standalone)

#### Scope-Based Validation
- **PCSG Members**: Validate role uniqueness within each PodCliqueScalingGroup
- **Standalone PodCliques**: Validate role uniqueness among all standalone PodCliques in PodGangSet
- **Cross-Reference**: Ensure scaling group references point to valid PodCliques with roles

## Key Design Challenge: Role Uniqueness

### Problem with Current Proposals

You're absolutely right to point out the issue with having the same role (like "LEADER") in the same PCSG. This creates several problems:

1. **Environment Variable Conflicts**: Multiple PodCliques with `roleName: LEADER` in the same PCSG would create conflicting `LEADER_ADDR` variables
2. **Ambiguous Discovery**: Frameworks wouldn't know which "LEADER" to connect to
3. **DNS Conflicts**: Multiple leaders would try to claim the same DNS name pattern

### Proposed Solutions

#### Option A: Enforce Unique Roles Within PCSG
- **Rule**: Each role must be unique within a PodCliqueScalingGroup
- **Example**: Within same PCSG, use `PREFILL_LEADER`, `DECODE_LEADER`, `WORKER` instead of multiple `LEADER` roles
- **Validation**: Admission webhook rejects PodGangSets with duplicate roles in same PCSG

#### Option B: Hierarchical Role Naming
- **Rule**: Allow same base role but require disambiguation
- **Example**: `PREFILL_LEADER`, `DECODE_LEADER` for two leader types
- **Pattern**: `<component>_<role>` naming convention

#### Option C: Scoped Role Variables
- **Rule**: Use PodClique name as disambiguation
- **Example**: `PREFILL_ADDR`, `DECODE_ADDR` based on PodClique names rather than roles
- **Trade-off**: Loses role abstraction benefits

### Recommended Solution: Option A (Unique Roles)

We recommend **Option A** because it:
- ✅ **Eliminates Ambiguity**: Clear, unambiguous role identification
- ✅ **Framework Friendly**: Easy for frameworks to understand role relationships  
- ✅ **Scales Well**: Works for complex multi-leader scenarios
- ✅ **Validation Friendly**: Easy to validate and provide clear error messages

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