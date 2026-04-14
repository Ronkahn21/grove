//go:build e2e

package scale

// /*
// Copyright 2026 The Grove Authors.
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

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"github.com/ai-dynamo/grove/operator/e2e/diagnostics"
	"github.com/ai-dynamo/grove/operator/e2e/grove/config"
	"github.com/ai-dynamo/grove/operator/e2e/grove/workload"
	"github.com/ai-dynamo/grove/operator/e2e/k8s/resources"
	"github.com/ai-dynamo/grove/operator/e2e/log"
	"github.com/ai-dynamo/grove/operator/e2e/testctx"

	"github.com/ai-dynamo/grove/operator/e2e/measurement"
	"github.com/ai-dynamo/grove/operator/e2e/measurement/condition"
	"github.com/ai-dynamo/grove/operator/e2e/measurement/exporter"
)

// Logger for the scale tests.
var Logger = log.NewTestLogger(log.InfoLevel)

const (
	defaultPollInterval = 100 * time.Millisecond
	defaultTimeout      = 15 * time.Minute
	defaultWorkerNodes  = 100
	defaultNamespace    = "default"

	runIDTimeFormat   = "20060102-150405"
	outputResultsFile = "scale-test-results.json"
)

// scaleTestConfig defines parameters for a single scale test run.
type scaleTestConfig struct {
	Name         string
	WorkloadName string
	YAMLPath     string
	ExpectedPods int
	PCSReplicas  int
	PCSCount     int
	WorkerNodes  int
	Timeout      time.Duration
	PollInterval time.Duration

	// Phases defines the test phases to execute. If nil, default deploy+delete phases are used.
	Phases func(ctx context.Context, tc *testctx.TestContext) []measurement.PhaseDefinition
}

// toOperatorMetadata converts GroveMetadata to the measurement package type.
func toOperatorMetadata(m *config.GroveMetadata) *measurement.OperatorMetadata {
	return &measurement.OperatorMetadata{
		GroveImage: m.Image,
		K8sClient: &measurement.K8sClientConfig{
			QPS:   m.Config.ClientConnection.QPS,
			Burst: m.Config.ClientConnection.Burst,
		},
		ControllerMaxReconcile: &measurement.ControllerMaxReconcile{
			PodCliqueSet:          ptr.Deref(m.Config.Controllers.PodCliqueSet.ConcurrentSyncs, 1),
			PodCliqueScalingGroup: ptr.Deref(m.Config.Controllers.PodCliqueScalingGroup.ConcurrentSyncs, 1),
			PodClique:             ptr.Deref(m.Config.Controllers.PodClique.ConcurrentSyncs, 1),
		},
	}
}

// defaultPhases returns the standard deploy+delete phases for a scale test.
func defaultPhases(_ context.Context, tc *testctx.TestContext) []measurement.PhaseDefinition {
	return []measurement.PhaseDefinition{
		{
			Name: "deploy",
			ActionFn: func(ctx context.Context) error {
				_, err := resources.NewResourceManager(tc.Clients, Logger).ApplyYAMLFile(ctx, tc.Workload.YAMLPath, tc.Namespace)
				return err
			},
			Milestones: []measurement.MilestoneDefinition{
				{
					Name: "pods-created",
					Condition: &condition.PodsCreatedCondition{
						Client:        tc.Clients.CRClient,
						Namespace:     tc.Namespace,
						LabelSelector: tc.GetLabelSelector(),
						ExpectedCount: tc.Workload.ExpectedPods,
					},
				},
				{
					Name: "pods-ready",
					Condition: &condition.PodsReadyCondition{
						Client:        tc.Clients.CRClient,
						Namespace:     tc.Namespace,
						LabelSelector: tc.GetLabelSelector(),
						ExpectedCount: tc.Workload.ExpectedPods,
					},
				},
				{
					Name: "pcs-available",
					Condition: &condition.PCSAvailableCondition{
						Client:        tc.Clients.CRClient,
						Name:          tc.Workload.Name,
						Namespace:     tc.Namespace,
						ExpectedCount: 1,
					},
				},
			},
		},
		{
			Name: "delete",
			ActionFn: func(ctx context.Context) error {
				return workload.NewWorkloadManager(tc.Clients, Logger).DeletePCS(ctx, tc.Namespace, tc.Workload.Name)
			},
			Milestones: []measurement.MilestoneDefinition{
				{
					Name: "pcs-deleted",
					Condition: &condition.PCSDeletedCondition{
						Client:    tc.Clients.CRClient,
						Name:      tc.Workload.Name,
						Namespace: tc.Namespace,
					},
				},
			},
		},
	}
}

// runScaleTest is the shared test runner for all scale test configurations.
func runScaleTest(t *testing.T, cfg scaleTestConfig) {
	t.Helper()

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	workerNodes := cfg.WorkerNodes
	if workerNodes == 0 {
		workerNodes = defaultWorkerNodes
	}
	pcsCount := cfg.PCSCount
	if pcsCount == 0 {
		pcsCount = 1
	}

	diagDir := os.Getenv(diagnostics.DirEnvVar)
	Logger.Infof("starting scale test %s: %d expected pods, timeout %v", cfg.Name, cfg.ExpectedPods, timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	Logger.Infof("preparing test cluster with %d worker nodes", workerNodes)
	tc, cleanup := testctx.PrepareTest(ctx, t, workerNodes,
		testctx.WithTimeout(timeout),
		testctx.WithInterval(pollInterval),
		testctx.WithWorkload(&testctx.WorkloadConfig{
			Name:         cfg.WorkloadName,
			YAMLPath:     cfg.YAMLPath,
			Namespace:    defaultNamespace,
			ExpectedPods: cfg.ExpectedPods,
		}),
	)
	defer cleanup()

	metadata, err := config.NewOperatorConfig(tc.Clients).ReadGroveMetadata(ctx)
	if err != nil {
		t.Fatalf("failed to read grove metadata: %v", err)
	}

	runID := fmt.Sprintf("run-%s", time.Now().Format(runIDTimeFormat))
	Logger.Infof("test config: runID=%s, namespace=%s, pcsName=%s", runID, tc.Namespace, tc.Workload.Name)

	outputDir := filepath.Join(cfg.Name, runID)
	if diagDir != "" {
		outputDir = filepath.Join(diagDir, cfg.Name, runID)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output directory: %v", err)
	}

	pprofOpt, pprofCleanup := setupPprofHook(ctx, tc.Clients, runID, outputDir, loadPyroscopeConfig())
	defer pprofCleanup()

	opts := []measurement.TimelineOption{
		measurement.WithPollInterval(pollInterval),
		measurement.WithLogger(Logger.GetLogr()),
	}
	if pprofOpt != nil {
		opts = append(opts, pprofOpt)
	}

	tracker := measurement.NewTimelineTracker(cfg.Name, runID, tc.Namespace, pcsCount, opts...)

	phaseFn := cfg.Phases
	if phaseFn == nil {
		phaseFn = defaultPhases
	}
	for _, phase := range phaseFn(ctx, tc) {
		tracker.AddPhase(phase)
	}

	Logger.Info("running timeline tracker")
	result, err := tracker.Run(ctx, toOperatorMetadata(metadata))
	if err != nil {
		t.Fatalf("Timeline tracker run failed: %v", err)
	}
	tracker.Wait()

	Logger.Info("exporting results")
	exportResult(t, result, outputDir)
	Logger.Infof("scale test %s completed successfully in %.1fs", cfg.Name, result.TestDurationSeconds)
}

// exportResult writes test results to JSON and stdout summary.
func exportResult(t *testing.T, result *measurement.TrackerResult, outputDir string) {
	t.Helper()

	outputPath := filepath.Join(outputDir, outputResultsFile)
	jsonFile, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("Failed to create JSON output file: %v", err)
	}
	defer jsonFile.Close()

	multi := exporter.NewMultiExporter(
		exporter.NewSummaryExporter(os.Stdout),
		exporter.NewJSONExporter(jsonFile),
	)

	if err := multi.Export(result); err != nil {
		t.Fatalf("Failed to export results: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test cases: vary pod counts
// ---------------------------------------------------------------------------

// Test_Scale_500 deploys 500 pods (250 replicas × 2 pods).
func Test_Scale_500(t *testing.T) {
	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_500",
		WorkloadName: "scale-test-500",
		YAMLPath:     "../../yaml/scale-test-500.yaml",
		ExpectedPods: 500,
		PCSReplicas:  250,
	})
}

// Test_Scale_1000 deploys 1000 pods (500 replicas × 2 pods).
func Test_Scale_1000(t *testing.T) {
	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_1000",
		WorkloadName: "scale-test-1000",
		YAMLPath:     "../../yaml/scale-test-1000.yaml",
		ExpectedPods: 1000,
		PCSReplicas:  500,
	})
}

// Test_Scale_5000 deploys 5000 pods (2500 replicas × 2 pods).
func Test_Scale_5000(t *testing.T) {
	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_5000",
		WorkloadName: "scale-test-5000",
		YAMLPath:     "../../yaml/scale-test-5000.yaml",
		ExpectedPods: 5000,
		PCSReplicas:  2500,
		Timeout:      30 * time.Minute,
	})
}

// ---------------------------------------------------------------------------
// Test cases: vary workload types
// ---------------------------------------------------------------------------

// Test_Scale_1000_MoE deploys 1000 pods using the MoE (Mixture of Experts) workload.
func Test_Scale_1000_MoE(t *testing.T) {
	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_1000_MoE",
		WorkloadName: "scale-test-1000-moe",
		YAMLPath:     "../../yaml/scale-test-1000-moe.yaml",
		ExpectedPods: 1000,
		PCSReplicas:  500,
	})
}

// ---------------------------------------------------------------------------
// Test cases: disaggregated inference at scale
// ---------------------------------------------------------------------------

// Test_Scale_Disagg_1k deploys a 1,000-pod disaggregated inference workload:
// 50 PCS replicas × 1 PCSG (inference-group) × 2 cliques (prefill + decode) × 10 pods each.
// Measures deploy time, then scales PCS replicas from 50 → 100 (2k pods),
// scales back to 50 (1k pods), and deletes.
func Test_Scale_Disagg_1k(t *testing.T) {
	const (
		initialReplicas = 1
		scaledReplicas  = 2
		podsPerReplica  = 1000                             // 2 cliques × 10 pods each
		initialPods     = initialReplicas * podsPerReplica // 1,000
		scaledPods      = scaledReplicas * podsPerReplica  // 2,000
	)

	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_Disagg_1k",
		WorkloadName: "scale-test-disagg-1k",
		YAMLPath:     "../../yaml/scale-test-disagg-1k.yaml",
		ExpectedPods: initialPods,
		PCSReplicas:  initialReplicas,
		Phases: func(_ context.Context, tc *testctx.TestContext) []measurement.PhaseDefinition {
			wm := workload.NewWorkloadManager(tc.Clients, Logger)
			rm := resources.NewResourceManager(tc.Clients, Logger)
			return []measurement.PhaseDefinition{
				{
					Name: "deploy",
					ActionFn: func(ctx context.Context) error {
						_, err := rm.ApplyYAMLFile(ctx, tc.Workload.YAMLPath, tc.Namespace)
						return err
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, initialPods),
						pcsAvailableMilestone(tc),
					},
				},
				{
					Name: "scale-up",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, scaledReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, scaledPods),
					},
				},
				{
					Name: "scale-down",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, initialReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, initialPods),
					},
				},
				{
					Name: "delete",
					ActionFn: func(ctx context.Context) error {
						return wm.DeletePCS(ctx, tc.Namespace, tc.Workload.Name)
					},
					Milestones: []measurement.MilestoneDefinition{
						pcsDeletedMilestone(tc),
					},
				},
			}
		},
	})
}

// Test_Scale_Disagg_5k deploys a 5,000-pod disaggregated inference workload:
// 250 PCS replicas × 1 PCSG (inference-group) × 2 cliques (prefill + decode) × 10 pods each.
// Measures deploy time, then scales PCS replicas from 250 → 500 (10k pods),
// scales back to 250 (5k pods), and deletes.
func Test_Scale_Disagg_5k(t *testing.T) {
	const (
		initialReplicas = 250
		scaledReplicas  = 500
		podsPerReplica  = 20                               // 2 cliques × 10 pods each
		initialPods     = initialReplicas * podsPerReplica // 5,000
		scaledPods      = scaledReplicas * podsPerReplica  // 10,000 (temporarily during scale-up)
	)

	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_Disagg_5k",
		WorkloadName: "scale-test-disagg-5k",
		YAMLPath:     "../../yaml/scale-test-disagg-5k.yaml",
		ExpectedPods: initialPods,
		PCSReplicas:  initialReplicas,
		PCSCount:     1,
		WorkerNodes:  defaultWorkerNodes,
		Timeout:      30 * time.Minute,
		Phases: func(_ context.Context, tc *testctx.TestContext) []measurement.PhaseDefinition {
			wm := workload.NewWorkloadManager(tc.Clients, Logger)
			rm := resources.NewResourceManager(tc.Clients, Logger)
			return []measurement.PhaseDefinition{
				{
					Name: "deploy",
					ActionFn: func(ctx context.Context) error {
						_, err := rm.ApplyYAMLFile(ctx, tc.Workload.YAMLPath, tc.Namespace)
						return err
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-created", tc, initialPods),
						podsMilestone("pods-ready", tc, initialPods),
						pcsAvailableMilestone(tc),
					},
				},
				{
					Name: "scale-up",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, scaledReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, scaledPods),
					},
				},
				{
					Name: "scale-down",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, initialReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, initialPods),
					},
				},
				{
					Name: "delete",
					ActionFn: func(ctx context.Context) error {
						return wm.DeletePCS(ctx, tc.Namespace, tc.Workload.Name)
					},
					Milestones: []measurement.MilestoneDefinition{
						pcsDeletedMilestone(tc),
					},
				},
			}
		},
	})
}

// ---------------------------------------------------------------------------
// Test cases: scale up/down cycles
// ---------------------------------------------------------------------------

// Test_Scale_UpDown_1000 deploys 500 pods, scales up to 1000, then scales back to 500,
// measuring time-to-ready at each step.
func Test_Scale_UpDown_1000(t *testing.T) {
	const (
		initialReplicas = 250
		scaledReplicas  = 500
		initialPods     = 500
		scaledPods      = 1000
	)

	runScaleTest(t, scaleTestConfig{
		Name:         "Scale_UpDown_1000",
		WorkloadName: "scale-test-1000",
		YAMLPath:     "../../yaml/scale-test-1000.yaml",
		ExpectedPods: initialPods,
		PCSReplicas:  initialReplicas,
		Phases: func(_ context.Context, tc *testctx.TestContext) []measurement.PhaseDefinition {
			wm := workload.NewWorkloadManager(tc.Clients, Logger)
			rm := resources.NewResourceManager(tc.Clients, Logger)
			return []measurement.PhaseDefinition{
				{
					Name: "deploy",
					ActionFn: func(ctx context.Context) error {
						_, err := rm.ApplyYAMLFile(ctx, tc.Workload.YAMLPath, tc.Namespace)
						return err
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, initialPods),
						pcsAvailableMilestone(tc),
					},
				},
				{
					Name: "scale-up",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, scaledReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, scaledPods),
					},
				},
				{
					Name: "scale-down",
					ActionFn: func(ctx context.Context) error {
						return wm.ScalePCS(ctx, tc.Namespace, tc.Workload.Name, initialReplicas)
					},
					Milestones: []measurement.MilestoneDefinition{
						podsMilestone("pods-ready", tc, initialPods),
					},
				},
				{
					Name: "delete",
					ActionFn: func(ctx context.Context) error {
						return wm.DeletePCS(ctx, tc.Namespace, tc.Workload.Name)
					},
					Milestones: []measurement.MilestoneDefinition{
						pcsDeletedMilestone(tc),
					},
				},
			}
		},
	})
}

// ---------------------------------------------------------------------------
// Milestone helpers
// ---------------------------------------------------------------------------

// podsMilestone creates a milestone that waits for expectedCount pods to be ready.
func podsMilestone(name string, tc *testctx.TestContext, expectedCount int) measurement.MilestoneDefinition {
	return measurement.MilestoneDefinition{
		Name: name,
		Condition: &condition.PodsReadyCondition{
			Client:        tc.Clients.CRClient,
			Namespace:     tc.Namespace,
			LabelSelector: tc.GetLabelSelector(),
			ExpectedCount: expectedCount,
		},
	}
}

// pcsAvailableMilestone creates a milestone that waits for the PCS to be available.
func pcsAvailableMilestone(tc *testctx.TestContext) measurement.MilestoneDefinition {
	return measurement.MilestoneDefinition{
		Name: "pcs-available",
		Condition: &condition.PCSAvailableCondition{
			Client:        tc.Clients.CRClient,
			Name:          tc.Workload.Name,
			Namespace:     tc.Namespace,
			ExpectedCount: 1,
		},
	}
}

// pcsDeletedMilestone creates a milestone that waits for the PCS to be deleted.
func pcsDeletedMilestone(tc *testctx.TestContext) measurement.MilestoneDefinition {
	return measurement.MilestoneDefinition{
		Name: "pcs-deleted",
		Condition: &condition.PCSDeletedCondition{
			Client:    tc.Clients.CRClient,
			Name:      tc.Workload.Name,
			Namespace: tc.Namespace,
		},
	}
}
