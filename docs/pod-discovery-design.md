# Pod Discovery Implementation in Grove

## Overview

Pod discovery is critical for multinode inference workloads where distributed AI models require pods to locate and communicate with their peers. This document describes Grove's current implementation of pod discovery through environment variables and index-based hostname management, highlighting key architectural decisions and unresolved design challenges.

## Original Requirements

The original specification defined a comprehensive pod discovery system:

### Environment Variables
Each pod receives environment variables identifying its position in the Grove hierarchy:

**Base Variables (All Pods):**
- `GROVE_PGS_NAME=<pgs-name>` - PodGangSet name (e.g., `inf`)
- `GROVE_PGS_INDEX=<pgs-index>` - PodGangSet replica index (e.g., `0`)
- `GROVE_PCLQ_NAME=<pclq-name>` - PodClique name (e.g., `inf-0-decode-0-leader`)
- `GROVE_PCLQ_POD_INDEX=<pclq-pod-replica-index>` - Pod index within PodClique (e.g., `0`)
- `GROVE_HEADLESS_SERVICE=<service-fqdn>` - Headless service FQDN (e.g., `inf0.dynamo.svc.cluster.local`)

**PCSG-Specific Variables (ScalingGroup Members):**
- `GROVE_PCSG_NAME=<pcsg-name>` - PodCliqueScalingGroup name (e.g., `inf-0-decode`)
- `GROVE_PCSG_TEMPLATE_PODS_PER_REPLICA=<count>` - Total pods at startup per PCSG replica (e.g., `5`)
- `GROVE_PCSG_INDEX=<pcsg-index>` - ScalingGroup replica index (e.g., `0`)

### Hostname Discovery
Pods must be discoverable via predictable hostnames through Kubernetes headless services, enabling frameworks to construct peer addresses programmatically.

## Current Implementation Status

### ✅ Implemented in This PR
1. **Basic Environment Variables**: `GROVE_PGS_NAME`, `GROVE_PGS_INDEX`, `GROVE_PCLQ_NAME`, `GROVE_PCLQ_INDEX`, `GROVE_HEADLESS_SERVICE`
2. **PCSG Variables**: `GROVE_PCSG_NAME`, `GROVE_PCSG_INDEX`, `GROVE_PCSG_TEMPLATE_PODS_PER_REPLICA`
3. **Hostname Configuration**: 
   - `pod.Spec.Hostname` set to `{pclq-name}-{index}` format
   - `pod.Spec.Subdomain` set to headless service name
   - Enables DNS resolution: `hostname.subdomain.namespace.svc.cluster.local`
4. **Index Management with Gap Filling**: 
   - Automatic assignment of lowest available index (0 to replicaSize-1)
   - Gap filling when pods are deleted and recreated
   - Index persistence via pod hostname storage
   - Index recovery from existing pod hostnames during initialization

### ❌ Missing from Original Requirements
1. **`GROVE_PCLQ_POD_INDEX`** - May not be required for each pod (unclear if really needed)

## Remaining Open Questions

While the core index management system is implemented and working well, there are a few remaining edge cases and requirement clarifications needed:

### 1. Invalid Index Pod Handling
**The main remaining design question**: What should happen to pods with invalid indices?

**Current Behavior:**
- Pods with malformed hostnames (can't extract index): Logged and skipped during initialization
- Pods with out-of-range indices: Logged and skipped during initialization
- Invalid pods continue running but are not included in index management

**How Out-of-Range Indices Occur:**
Out-of-range indices are a legitimate operational concern that can happen in several scenarios:

1. **Scaling Down Operations**: 
   - PodClique scaled from 5 to 3 replicas, but existing pods with indices 3, 4 haven't been cleaned up yet
   - Pods with indices outside the new replica range become "orphaned"

2. **Manual Pod Deletion Edge Cases**:
   - Admin deletes multiple pods manually without using Grove controllers
   - Some high-index pods survive while lower-index pods are removed, creating range gaps


**Open Question:**
Should these invalid pods be automatically deleted and recreated with valid indices, or left running as-is?

**Trade-offs:**
- **Delete & Recreate**: Ensures all pods have valid indices, but may disrupt running workloads
- **Leave Running**: Preserves existing workloads, but creates inconsistent state

### 2. Environment Variable Requirements Clarification
**Question**: Is `GROVE_PCLQ_POD_INDEX` actually required for each pod?


## Current Implementation Approach

### Index Management Strategy (Implemented)
**Features:**
- **Gap Filling**: Automatically assigns lowest available index (0 to replicaSize-1)
- **Index Persistence**: Stores indices in pod hostnames for recovery across restarts
- **Automatic Recovery**: Rebuilds index state from existing pod hostnames during initialization

**Benefits:**
- Consistent index assignments across pod recreations
- Robust recovery from controller restarts
- Simple and predictable behavior

**Current Limitations:**
- Invalid index pods are ignored rather than automatically remediated
- No special handling for scaling edge cases (mainly affects standalone PodCliques)

## Scope Limitations

The current implementation primarily affects **standalone PodCliques**.
ScalingGroup PodCliques are mostly covered from experience the side effects of the current implementation