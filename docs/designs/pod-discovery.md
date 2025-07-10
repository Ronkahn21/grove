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

### Approach 1: External Service Discovery (Rejected)

This approach would use external service discovery mechanisms like etcd, Consul, or Kubernetes Services with custom controllers.

**Implementation:**
- Deploy external service registry (etcd/Consul)
- Create sidecar containers for service registration
- Use custom controllers to populate service information
- Frameworks query external registry for peer discovery

**Pros:**
- Rich query capabilities and dynamic updates
- Well-established patterns in distributed systems
- Can support complex service topologies

**Cons:**
- ❌ **Additional Infrastructure**: Requires deploying and managing additional services
- ❌ **Framework Dependencies**: Requires frameworks to integrate with specific service discovery clients
- ❌ **Network Dependencies**: Fails during network partitions or service outages
- ❌ **Complexity**: Increases operational overhead and debugging complexity
- ❌ **Startup Dependencies**: Pods cannot start until service discovery is available
- ❌ **Not Kubernetes Native**: Doesn't align with Kubernetes' declarative model

### Approach 2: Environment Variable-Based Discovery (Recommended)

This approach uses environment variables injected by Grove operator to provide role-based discovery information.

**Implementation:**
- Add `roleName` field to PodCliqueSpec for role identification
- Grove operator discovers all roles within PodGangSet scope
- Inject environment variables into each pod with discovery information
- Frameworks read environment variables for peer discovery

**Pros:**
- ✅ **Kubernetes Native**: Follows established Kubernetes patterns
- ✅ **Zero Dependencies**: No external services or agents required
- ✅ **Framework Agnostic**: Works with any containerized application
- ✅ **Immediate Availability**: Available at pod startup
- ✅ **Reliable**: Immune to network issues or service outages
- ✅ **Simple Operations**: No additional infrastructure to deploy or manage
- ✅ **Grove Integration**: Leverages existing PodGangSet architecture

**Cons:**
- ❌ **Static Information**: Cannot be updated without pod restart
- ❌ **Limited Query Capabilities**: Simple key-value pairs only

## Recommended Approach: Environment Variables

We recommend **Approach 2 (Environment Variables)** because it aligns with Grove's design principles and inference workload requirements:

### Why Environment Variables Win for Inference

1. **Inference Workload Patterns**: Most inference serving is long-running with stable pod membership
2. **Framework Compatibility**: All major inference frameworks (TensorRT-LLM, vLLM, SGLang) support environment variables
3. **Kubernetes Alignment**: Matches patterns used by StatefulSets, Jobs, and other core Kubernetes resources
4. **Operational Simplicity**: Reduces moving parts and operational complexity
5. **Grove Architecture Fit**: Leverages existing PodGangSet scope and gang scheduling

## Detailed Design

### 1. Role-Based Environment Variable Injection

#### Core Environment Variables
Each pod will receive standard environment variables:

```bash
# Pod's own role information
GROVE_ROLE=<role-name>
GROVE_CLIQUE_NAME=<podclique-template-name>
GROVE_REPLICA_INDEX=<pod-replica-index>
GROVE_PGS_REPLICA_INDEX=<podgangset-replica-index>

# Leadership information (for replica index 0 of each role)
GROVE_IS_LEADER=true|false
```

#### Service Discovery Variables
For each role in the PodGangSet, pods receive discovery information:

```bash
# Service discovery for each role
GROVE_ROLE_<ROLE_NAME>_COUNT=<replica-count>
# Additional discovery variables to be determined based on implementation needs
```

### 2. Multi-Leader Support Within PodGangSet

#### Scenario 1: Multiple Leader Types in Same Scaling Group
In a single PodCliqueScalingGroup with multiple leader types:

```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
metadata:
  name: llm-inference
spec:
  template:
    cliques:
      - name: prefill-leaders
        spec:
          roleName: prefill-leader
          replicas: 2
          podSpec: # ... pod specification
      - name: decode-leaders  
        spec:
          roleName: decode-leader
          replicas: 2
          podSpec: # ... pod specification
      - name: workers
        spec:
          roleName: worker
          replicas: 4
          podSpec: # ... pod specification
    podCliqueScalingGroups:
      - name: inference-group
        # Multiple leader types in the SAME scaling group
        cliqueNames: ["prefill-leaders", "decode-leaders", "workers"]
```

#### Scenario 2: Mixed - Scaling Groups + Standalone PodCliques
```yaml
apiVersion: grove.io/v1alpha1
kind: PodGangSet
metadata:
  name: multi-model-inference
spec:
  template:
    cliques:
      - name: text-leaders
        spec:
          roleName: text-leader
          replicas: 2
          podSpec: # ... pod specification
      - name: vision-leaders
        spec:
          roleName: vision-leader
          replicas: 2
          podSpec: # ... pod specification
      - name: inference-workers
        spec:
          roleName: worker
          replicas: 8
          podSpec: # ... pod specification
      - name: standalone-gateway
        spec:
          roleName: api-gateway
          replicas: 1
          podSpec: # ... pod specification
    podCliqueScalingGroups:
      - name: inference-group
        # Multiple leaders + workers in same scaling group
        cliqueNames: ["text-leaders", "vision-leaders", "inference-workers"]
    # standalone-gateway remains outside scaling groups
```

#### Environment Variables for Multi-Leader Scenarios

**Scenario 1: Multiple Leaders in Same Scaling Group**
For a worker pod in the `inference-group` scaling group:

```bash
# Pod's own information
GROVE_ROLE=worker
GROVE_CLIQUE_NAME=workers
GROVE_REPLICA_INDEX=2
GROVE_IS_LEADER=false

# Discover prefill-leader role (in same scaling group)
GROVE_ROLE_PREFILL_LEADER_COUNT=2

# Discover decode-leader role (in same scaling group)
GROVE_ROLE_DECODE_LEADER_COUNT=2

# Discover worker role (own role, in same scaling group)
GROVE_ROLE_WORKER_COUNT=4
```

**Scenario 2: Mixed Scaling Groups + Standalone**
For a worker pod in the `inference-group` scaling group:

```bash
# Pod's own information
GROVE_ROLE=worker
GROVE_CLIQUE_NAME=inference-workers
GROVE_REPLICA_INDEX=3
GROVE_IS_LEADER=false

# Leaders in same scaling group
GROVE_ROLE_TEXT_LEADER_COUNT=2
GROVE_ROLE_VISION_LEADER_COUNT=2

# Own role in scaling group
GROVE_ROLE_WORKER_COUNT=8

# Standalone gateway (outside scaling groups)
GROVE_ROLE_API_GATEWAY_COUNT=1
```

### 3. Implementation Architecture

#### Component Modifications

**1. Pod Creation Enhancement**
- Location: `internal/component/pclq/pod/pod.go`
- Modify `buildResource()` to inject role-based environment variables
- Extract role information from PodClique labels and metadata

**2. Environment Variable Provider**
- New component: `internal/component/pclq/pod/discovery.go`
- Responsible for generating role-based environment variables
- Query PodGangSet to discover all roles across scaling groups and standalone PodCliques
- Handle cross-scaling-group role discovery

**3. Service Discovery Integration**
- Enhance existing headless service patterns
- Add role-based service naming conventions
- Maintain backward compatibility with current service discovery

#### Environment Variable Generation Flow

```
PodGangSet (contains PodCliqueScalingGroups + standalone PodCliques)
    ↓
Discover all PodCliques within the PodGangSet scope
    ↓ 
Group PodCliques by role across all scaling groups and standalone cliques
    ↓
Generate environment variables for cross-role discovery
    ↓
Inject into pod containers during creation
```

### 4. Naming Conventions

#### Environment Variable Naming
- Use uppercase with underscores: `GROVE_ROLE_<ROLE>_<ATTRIBUTE>`
- Role names converted to uppercase: `prefill-leader` → `PREFILL_LEADER`
- Consistent with Kubernetes environment variable patterns

#### Service Naming
- Leverage existing headless service patterns
- Maintain compatibility with current DNS patterns
- Specific service naming conventions to be determined based on implementation needs

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