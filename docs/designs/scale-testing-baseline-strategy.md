# 🚀 Grove Scale Testing Infrastructure: Proposal

> **Status:** Draft (pre-KEP)
> **Purpose:** Proposed approach for scale testing infrastructure
> **Audience:** Grove engineering team

---

## 1. 🎯 Executive Summary

### 1.1 Overview

This document proposes an approach for scale testing Grove operator infrastructure. It covers:
- Recommended infrastructure options (KWOK vs Kubemark)
- Suggested testing patterns and scenarios
- Observability and profiling recommendations
- Practical implementation examples

### 1.2 Proposed Approach

**Infrastructure Recommendations:**
- Suggested: KWOK clusters for scale testing (primary)
- Alternative: Kubemark for specific scenarios (startup ordering)
- Control plane requirements and considerations

**Test Patterns:**
- Recommended test pattern structures
- Metrics to collect
- Example test implementations

**Observability:**
- Suggested instrumentation for profiling
- Recommended metrics to track
- Performance analysis approaches

**Version Tracking:**
- Approaches for tracking performance across versions
- Suggested regression detection with benchstat
- Trend visualization options

### 1.3 Grove's Testing Focus

**⚠️ Recommended focus: POD CREATION, not pod scheduling.**

Grove's primary responsibility is creating Pod objects in the API server. Suggested measurement:
- **Recommended metric:** Time from PCS creation → Pod objects exist in API server
- **Not prioritized:** Scheduling latency, pod startup time, container execution

**Why KWOK is recommended:** Pods transition to "Running" instantly, isolating operator performance from scheduling noise.

---

## 2. 🏗️ Infrastructure Recommendations

### 2.1 Simulated Node Strategy

Suggested approach: Decouple operator testing from infrastructure constraints using simulated nodes.

### Option A: KWOK (Kubernetes WithOut Kubelet) — RECOMMENDED ✅

**Why recommended for Grove:**
- Instant pod transitions → isolates operator's pod creation performance
- Lightweight: simulate 10,000+ pods on a single machine
- Pods go "Running" immediately (no actual containers)
- Perfect for measuring: "Time from PCS creation → Pod objects exist in API server"

**Best for:**
- Measuring pure reconciliation/creation latency
- Cascade operation timing (PCS → PCSG → PCLQ → Pods)
- API server stress testing
- Finding the "knee" where performance breaks

**Limitations:**
- No actual container runtime (but we don't need it for 90% of scale tests)
- Can't test real networking or storage behavior

**Setup:**
```bash
# Create 1000-node cluster in < 1 minute
kwokctl create cluster --name grove-scale --nodes 1000
```

### Option B: Kubemark — SPECIFIC USE CASES

**Use when:**
- Testing startup ordering at scale (CliqueStartupType sequencing)
- Validating phase-dependent logic: Phase1 → Phase2 → Phase3
- Need realistic kubelet timing behavior

**Best for:**
- Startup ordering edge cases
- Timing-sensitive sequential operations

**Limitations:**
- Heavier than KWOK (~50MB per node vs 10MB)
- Slower setup (5-10 minutes vs < 1 minute)

### KWOK vs Kubemark: Decision Matrix

| Aspect                  | KWOK                                   | Kubemark                         |
|-------------------------|----------------------------------------|----------------------------------|
| **Setup Speed**         | < 1 min for 1000 nodes                 | 5-10 min for 1000 nodes          |
| **Resource Usage**      | ~10MB per node                         | ~50MB per node                   |
| **Pod Transitions**     | Instant ✨                             | Realistic timing ⏲️              |
| **Best Use Case**       | Pod creation performance, cascade ops  | Startup ordering sequencing      |
| **Grove Primary Tests** | ✅ All discovery scenarios             | Startup ordering edge cases only |

### Suggested Approach

- **Recommended: KWOK for 90% of scale testing** — measures operator's core performance
- **Alternative: Kubemark for startup ordering** — for testing sequential phase dependencies


## 3. 🔍 Observability Recommendations

### 3.1 Current Grove Capabilities

Grove exposes controller-runtime metrics on **port 9445**:

```
workqueue_depth{name="podcliqueset"}
workqueue_adds_total{name="podcliqueset"}
workqueue_retries_total{name="podcliqueset"}
controller_runtime_reconcile_total{controller="podcliqueset",result="success"}
controller_runtime_reconcile_time_seconds{controller="podcliqueset"}
```

**The Problem:** These tell us WHAT broke, not WHY.

### 3.2 Profiling: Understanding Performance Bottlenecks

Profiling is critical for understanding WHERE the operator spends time and memory during scale tests. Without profiling, we can observe THAT performance degraded, but not identify WHY or which code path is responsible.

#### Profiling Options Comparison

**pprof (Built-in Go Standard Library)**

Already integrated in Grove on port 2753. Provides on-demand CPU, memory, goroutine, mutex, and block profiles.

- **Strengths:** No dependencies, standard Go tooling, already available in Grove
- **Limitations:** Manual capture required (must remember to collect profiles), no automatic historical tracking
- **Best use case:** Ad-hoc investigation during development, quick bottleneck identification
- **When to use:** Initial scale testing, local debugging

**Parca (eBPF-based Profiling)**

Zero-instrumentation profiling using eBPF to sample running processes.

- **Strengths:** No code changes needed, works with any language, very low overhead
- **Limitations:** Requires Linux with eBPF support, more complex infrastructure, platform-specific
- **Best use case:** Production profiling where code modification isn't possible
- **When to use:** If Grove is deployed in environments where you can't control the binary

**Pyroscope (Continuous Profiling Platform)**

Application-integrated continuous profiling with automatic collection and visualization.

- **Strengths:** Automatic profile collection throughout test runs, built-in comparison UI, tracks performance over time, low overhead (<2%), excellent for regression detection
- **Limitations:** Requires server deployment, SDK integration in code
- **Best use case:** Long-running scale tests, tracking performance across git commits, automated regression detection
- **When to use:** When establishing performance baselines and tracking trends

#### Recommended Approach

**Phase 1 (Immediate):** Use Grove's existing pprof support for initial scale testing. Manually capture profiles during test runs to identify obvious bottlenecks.

**Phase 2 (Continuous Testing):** Add Pyroscope integration when running automated nightly scale tests. This enables automatic profile comparison across versions without manual intervention.

pprof provides sufficient capability for understanding operator behavior. Pyroscope becomes valuable when you want to detect performance regressions automatically across many test runs.

### 3.3 Metrics Strategy: Instrumenting Grove Capabilities

Controller-runtime provides generic metrics for all Kubernetes operators (workqueue depth, reconciliation latency, error rates). While valuable, these don't capture Grove-specific operations.

#### The Instrumentation Pattern

**Concept:** Add custom metrics for each unique Grove capability. This enables understanding which Grove-specific features contribute to performance characteristics.

**Grove's Unique Capabilities to Instrument:**

**Multi-Layer Gang Coordination:**
Grove orchestrates scheduling across three layers (PCS → PCSG → PCLQ). The coordination overhead should be measurable to understand the cost of this abstraction versus flat pod deployment.

**Cascade Synchronization:**
Changes propagate through ownership hierarchies (4 levels deep in some cases). Measuring sync latency at each depth reveals where the reconciliation tree becomes expensive.

**Topology-Aware Scheduling:**
Grove evaluates complex spread constraints (zone, node, custom domains). For large pod groups, constraint evaluation can become a bottleneck.

#### Why Custom Metrics Matter

Controller-runtime's `reconcile_time_seconds` tells us the total reconciliation took 2 seconds, but doesn't explain whether that's from:
- Gang coordination logic (expensive label matching?)
- Cascade sync (deep tree traversal?)
- API server calls (rate limiting?)
- Topology evaluation (complex constraint checking?)

Custom metrics per capability enable precise bottleneck identification.

#### What Each Test Measures

The metrics collected depend on what the test validates. A depth test might focus on cascade sync metrics, while a topology test measures constraint evaluation. The infrastructure provides the instrumentation; each test selects relevant metrics to analyze.

### 3.4 The "Metrics Endpoint Stuck" Problem

At 10,000 objects, the `/metrics` endpoint itself can timeout (too much data to serialize).

**Solutions:**
1. Dedicated "Perf-Prometheus" with **60s scrape timeout** (vs default 10s)
2. Use **remote-write** to push metrics (don't rely on scraping)

### 3.5 Understanding Test Infrastructure Requirements

Not all scale tests require the same infrastructure. Understanding what each test actually validates determines the infrastructure needed.

#### Node Requirements: When Do We Need Them?

**Some tests don't need nodes at all:**

For tests validating **pod creation** (API object creation), nodes aren't required. The operator's responsibility is creating Pod and PodGang objects in the API server. What happens afterward (scheduling, pod startup) is handled by Kubernetes components.

Example: Non-elastic workloads where `minAvailable == replicas`:
- Validation: Verify PodGang and Pod objects exist in API server
- No scheduling required: Don't care if pods are Running
- Infrastructure needed: Just API server (can run without any nodes)

**When nodes are needed:**

Tests validating scheduling behavior, topology constraints, or actual pod lifecycle require nodes (real or simulated).

- Scheduling validation: Need nodes for kube-scheduler to place pods
- Topology testing: Need nodes with appropriate labels (zones, GPU domains)
- Startup ordering with timing: Need Kubemark for realistic kubelet behavior

#### CI vs Offloaded Testing

**CI-Friendly Tests (GitHub Actions):**
- Scale: 100-500 pods maximum
- Duration: < 15 minutes
- Purpose: Regression detection on every PR
- Infrastructure: Current e2e setup (28-node k3d) or lightweight KWOK
- Example: Depth test with 100 pods to verify cascade sync doesn't regress

**Offloaded Tests (Dedicated Infrastructure):**
- Scale: 1,000-10,000+ pods
- Duration: 30-60 minutes
- Purpose: Performance characterization, finding limits
- Infrastructure: KWOK on dedicated hardware or cloud clusters
- Example: Depth test with 10k pods to establish memory/latency characteristics
- Run frequency: Nightly or weekly

**Why this matters:** CI has resource constraints (runner CPU/memory/time limits). Large-scale tests should run on dedicated infrastructure to avoid blocking PR merges and to get consistent, reproducible results.

The infrastructure choice depends on what you're validating, not just the scale.

---


## 4. 📊 Version Trending: "Did We Get Faster or Slower?"

### 4.1 The Baseline Database

Store results for every test run to track performance across versions.

**Schema:**
```json
{
  "git_sha": "abc123",
  "date": "2024-01-15",
  "test": "depth-test-10k",
  "p50_latency_ms": 120,
  "p99_latency_ms": 450,
  "memory_mb": 1800,
  "cpu_cores": 1.2,
  "workqueue_max_depth": 280,
  "pod_creation_time_seconds": 45
}
```

**Storage Options:**

| Option | Pros | Cons | Best For |
|--------|------|------|----------|
| S3 + JSON files | Simple, no infrastructure | Manual querying | Quick start |
| InfluxDB | Time-series optimized | Requires setup | Better queries |
| InfluxDB + Grafana | Best visualization | Most complex | Production monitoring |

**Suggested:** Start with S3 + JSON, migrate to InfluxDB when you have 30+ test runs.

### 4.2 The Trend Dashboard

Visualize performance changes across commits:

```
┌─────────────────────────────────────────────────┐
│  Grove Performance Trends (Last 30 Days)        │
├─────────────────────────────────────────────────┤
│                                                  │
│  P99 Latency                   Memory Usage     │
│    500ms ┤                       2.0GB ┤         │
│         ┤    ●                        ┤  ●      │
│    400ms ┤   ●●                   1.5GB ┤ ●●     │
│         ┤  ●  ●                       ┤●  ●●    │
│    300ms ┤●●    ●●                1.0GB ┤    ● ● │
│         └────────────                  └────────│
│         v1.0  v1.1  v1.2              v1.0  v1.2│
│                                                  │
│  🔴 Regression Alert: P99 +30% in v1.2          │
└─────────────────────────────────────────────────┘
```

**Key visualizations:**
- P99 latency trend line
- Memory usage per 1000 pods
- Pod creation time (time to last pod created)
- Workqueue max depth during test

### 4.3 Regression Detection: benchstat

For Go benchmark tests (unit-level):

```bash
# Run benchmarks on main branch
git checkout main
go test -bench=. -benchtime=10s ./internal/controller/... > baseline.txt

# Run benchmarks on PR branch
git checkout feature-branch
go test -bench=. -benchtime=10s ./internal/controller/... > pr.txt

# Statistical comparison
benchstat baseline.txt pr.txt
```

**Output example:**
```
name                      old time/op  new time/op  delta
ReconcilePodCliqueSet-16    210µs ± 1%   273µs ± 2%  +30.0% (p=0.008 n=10+10)

🔴 REGRESSION: 30% slower!
```

**CI Integration:**
- Fail PR if > 15% regression without explanation
- Store benchmark results with git SHA
- Compare against last 10 main branch runs (not just immediate parent)

### 4.4 Public Results Publishing (Opensource Transparency)

**The Opensource Challenge:**
Grove is an opensource project, but scale tests may run on private NVIDIA infrastructure. How do we maintain transparency while protecting internal systems?

**Industry Pattern:**
Other opensource schedulers solve this by keeping test **code** public while running on private infrastructure:

| Project | Public | Private | Results Publishing |
|---------|--------|---------|-------------------|
| **Kubernetes** | Prow config, test code | GKE cluster running Prow | TestGrid (public dashboard) |
| **KAI Scheduler** | Scale test code in e2e/ | GitHub Actions in monorepo | S3 bucket → GitHub Pages |
| **Kueue** | Test scripts in repo | Execution environment | Results in issues/docs |
| **YuniKorn** | Scale test infrastructure | Jenkins jobs | Public docs with results |

**Suggested Approach for Grove:**

1. **Test Code:** Keep all scale test code in `operator/e2e/scale/` (public)
2. **Execution:** GitHub Actions can run on NVIDIA self-hosted runners (already exists)
3. **Results Publishing:**
   - Export results as JSON: `{git_sha, date, test_name, metrics}`
   - Push to public S3 bucket: `s3://grove-scale-tests/results/`
   - Or: GitHub Releases with JSON artifacts
4. **Visualization:**
   - Static GitHub Pages site at `grove-project.github.io/scale-tests`
   - Show trend graphs (last 30/90 days)
   - Display current performance characteristics

**Example from KAI Scheduler:**
```bash
# After test completes, publish results
aws s3 cp results.json s3://kai-public-results/scale-tests/${GIT_SHA}.json
# Generate static page with graph
python scripts/generate-trends.py --output docs/scale-results.html
```

**Benefits:**
- Community can see performance trends over time
- Results are reproducible (anyone can run same test code)
- Builds trust in Grove's scalability claims
- External contributors can validate their changes don't regress performance

---



---

## 5. 📚 Industry References & API Stress

### 6.1 How Other Projects Do It

#### Cluster-API (CAPI)
- **Scale:** Tests up to 1,000 clusters (each CAPI Cluster CR manages a full K8s cluster!)
- **Tools:** KWOK + ClusterLoader2
- **Key insight:** Workqueue depth is the #1 indicator of controller overload
- **Reference:** [kubernetes-sigs/cluster-api scale testing docs](https://cluster-api.sigs.k8s.io/)

#### Karmada (Multi-Cluster Orchestration)
- **Scale:** 100+ member clusters
- **Focus:** API server pressure monitoring
- **Key metric:** `apiserver_request_duration_seconds`
- **Learning:** Don't DDoS your own control plane

#### Argo CD
- **Scale:** 10,000 Application CRs across 100 clusters
- **Discovery:** Workqueue depth > 500 means you're falling behind
- **Solution:** Sharding across multiple controller replicas
- **Key insight:** Reconciliation debt compounds exponentially

### 6.2 API Server Pressure Monitoring

**The "Are We DDoS-ing the API Server?" Check:**

```promql
# Request rate from Grove operator
rate(apiserver_request_duration_seconds_count{
  job="apiserver",
  client="grove-operator",
  verb=~"GET|LIST|PATCH|CREATE|DELETE"
}[5m])

# If this goes > 100 req/s, we're hammering the API server
```

**Watch for:**
- 429 (Too Many Requests) responses
- P99 API latency > 1s (sign of overload)
- Increased etcd latency

**Client-Side Rate Limiting:**

Grove uses controller-runtime defaults (already in place):
```go
// Default: QPS=5, Burst=10 (very conservative!)
restConfig.QPS = 5
restConfig.Burst = 10

// For scale testing, increase:
restConfig.QPS = 50     // 10x higher
restConfig.Burst = 100  // 10x higher
```

**Suggested tuning:** Consider increasing these limits if API server throttling (429s) occurs during testing.

### 6.3 The "Informer Cache Warm-Up" Problem

When operator starts with 10,000 existing CRs:

1. **Informer does LIST** to get all objects (expensive!)
2. **Builds in-memory cache** (can take 30s for large datasets)
3. **Only then starts reconciling**

**Measure this:**
```
cache_warmup_duration = time_to_first_reconcile - operator_start_time
```

**Suggested targets:** < 10 seconds for 10k objects
**Investigate if:** > 60 seconds (may indicate inefficient serialization)

---

## 8. 🎬 Quick Start: Running Your First Discovery Test

### Prerequisites
- Docker installed
- `kubectl` configured
- `kwokctl` installed: `go install sigs.k8s.io/kwok/cmd/kwokctl@latest`
- Go 1.24.5+ (for running tests)

### Step 1: Deploy KWOK Cluster (30 seconds)

```bash
cd operator/e2e

# Create 1000-node cluster
kwokctl create cluster --name grove-scale --nodes 1000

# Verify
kubectl get nodes --no-headers | wc -l  # Should show 1000
```

### Step 2: Deploy Grove Operator with Profiling

```bash
# Build operator image
make docker-build IMG=grove-operator:test

# Deploy to KWOK cluster with profiling enabled
export ENABLE_PROFILING=true
export PYROSCOPE_SERVER_URL=http://pyroscope:4040
make deploy IMG=grove-operator:test

# Verify operator is running
kubectl get pods -n grove-system
```

### Step 3: Run Your First Scale Test

```bash
cd operator/e2e/tests

# Run the depth test (creates 1 PCS with 10,000 pods)
go test -v -run Test_Scale_Depth10k -timeout 60m

# In another terminal, watch metrics
kubectl port-forward -n grove-system svc/grove-operator-metrics 9445:9445
watch 'curl -s http://localhost:9445/metrics | grep workqueue_depth'
```

### Step 4: Capture Profiling Data

```bash
# Capture heap dump for analysis
kubectl port-forward -n grove-system pod/grove-operator-xxx 2753:2753
curl http://localhost:2753/debug/pprof/heap > heap-depth-10k.prof

# Analyze memory allocation
go tool pprof -top heap-depth-10k.prof
go tool pprof -web heap-depth-10k.prof  # Opens flame graph
```

### Step 5: View Results in Pyroscope (if integrated)

```bash
# Port-forward to Pyroscope
kubectl port-forward -n monitoring svc/pyroscope 4040:4040
open http://localhost:4040

# Compare profiles:
# - Select "grove-operator" application
# - Filter by test tag: test="depth-10k"
# - Compare profiles across different test runs
```

### Expected Results

After running the depth test, you should have:
- [ ] Pod creation time for 10k pods
- [ ] P99 reconciliation latency
- [ ] Memory usage at 10k pods
- [ ] Workqueue max depth during test
- [ ] CPU profile showing top functions
- [ ] Heap profile showing memory allocations

---

## 9. 🚦 Checklist: Scale Test Infrastructure Setup

### Infrastructure Complete When:

**Core Infrastructure:**
- [ ] **KWOK cluster setup automated**
  Can create 1000-node cluster in < 1 minute

- [ ] **Operator deployable with profiling**
  `make deploy` with ENABLE_PROFILING=true works

- [ ] **Metrics collection configured**
  Prometheus scraping operator metrics on port 9445

**Test Patterns Implemented:**
- [ ] **Depth test pattern**
  Can parameterize pod count (100, 1k, 10k)

- [ ] **Cold start test pattern**
  Can pre-create PCS and measure operator restart

- [ ] **Cascade delete test pattern**
  Can measure deletion time for large PCS

- [ ] **At least 3 patterns working end-to-end**
  Tests run, collect metrics, store results

**Observability:**
- [ ] **Test harness collects metrics**
  Pod creation time, P99 latency, memory, workqueue depth

- [ ] **Results stored with git SHA**
  Can track performance across versions

- [ ] **Basic profiling working**
  Can capture heap/cpu profiles during test runs

### Optional Enhancements:

**Advanced Infrastructure:**
- [ ] Kubemark setup for startup ordering tests
- [ ] Pyroscope integration for continuous profiling
- [ ] Profile comparison automation

**Automation:**
- [ ] CI integration for nightly scale tests
- [ ] Automated regression detection (benchstat)
- [ ] Grafana dashboards for trend visualization

### What This Infrastructure Enables:

Once complete, you can:
- Run scale tests at different sizes (100, 1k, 10k pods)
- Collect performance data (latency, memory, throughput)
- Profile operator behavior under load
- Track performance across code changes
- Identify performance regressions in CI

**Note:** The infrastructure teaches HOW to test. The actual limits (where things break, optimal configurations) will be discovered when you run the tests.

---

## 10. 📝 Using This Infrastructure

After building the scale test infrastructure:

**Immediate Use:**
1. **Run scale tests** at different sizes (100, 1k, 10k pods)
2. **Collect performance data** (latency, memory, throughput)
3. **Analyze profiles** to understand operator behavior
4. **Track metrics across versions** to detect regressions

**Future Enhancements:**
1. **Write KEP** documenting scale testing approach
2. **Set SLOs** based on collected performance data
3. **Add custom metrics** as identified during testing
4. **Integrate with CI** for automated regression detection
5. **Build dashboards** for ongoing monitoring
6. **Document tuning guide** for production deployments

---

## References

- [KWOK Documentation](https://kwok.sigs.k8s.io/)
- [Kubemark Guide](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-scalability/kubemark-guide.md)
- [Pyroscope Go Integration](https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/)
- [Controller-Runtime Metrics](https://book.kubebuilder.io/reference/metrics.html)
- [Cluster-API Scale Testing](https://cluster-api.sigs.k8s.io/)
- [benchstat Tool](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)

---

**Last Updated:** 2024-01-15
**Next Review:** After first baseline run
