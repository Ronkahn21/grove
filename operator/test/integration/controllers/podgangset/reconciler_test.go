package podgangset_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/test/integration/framework"
	"github.com/NVIDIA/grove/operator/test/utils"
)

func TestPodGangSetCreatesChildResources(t *testing.T) {
	// Setup test environment with PGS controller only
	env := framework.NewEnvBuilder(t).
		WithGroveCRDs().
		WithSchedulerCRDs().
		WithPodGangSetController().
		WithRBAC().
		WithNamespace("test-ns").
		Build()

	// Create a simple PGS with 2 cliques
	pgs := utils.NewPodGangSetBuilder("test-pgs", "test-ns").
		WithReplicas(1).
		WithPodCliqueParameters("clique-1", 2, nil).
		WithPodCliqueParameters("clique-2", 1, []string{"clique-1"}).
		Build()

	// Submit PGS to cluster
	err := env.Client.Create(env.Ctx, pgs)
	require.NoError(t, err)

	// Debug: Print the created PGS structure
	t.Logf("Created PGS spec.replicas: %d", pgs.Spec.Replicas)
	t.Logf("Created PGS spec.template.cliques count: %d", len(pgs.Spec.Template.Cliques))
	for i, clique := range pgs.Spec.Template.Cliques {
		t.Logf("Clique %d: name=%s, replicas=%d, startsAfter=%v", i, clique.Name, clique.Spec.Replicas, clique.Spec.StartsAfter)
	}

	// Debug: Check if PGS is actually in the cluster and monitor status changes
	time.Sleep(2 * time.Second)
	fetchedPGS := &grovecorev1alpha1.PodGangSet{}
	err = env.Client.Get(env.Ctx, client.ObjectKey{Name: "test-pgs", Namespace: "test-ns"}, fetchedPGS)
	require.NoError(t, err, "Should be able to fetch PGS from cluster")

	// Debug detailed status information
	t.Logf("Fetched PGS finalizers: %v", fetchedPGS.Finalizers)
	t.Logf("Fetched PGS status.replicas: %d", fetchedPGS.Status.Replicas)
	t.Logf("Fetched PGS status.updatedReplicas: %d", fetchedPGS.Status.UpdatedReplicas)
	if fetchedPGS.Status.ObservedGeneration != nil {
		t.Logf("Fetched PGS status.observedGeneration: %d", *fetchedPGS.Status.ObservedGeneration)
	}

	// Debug LastOperation details
	if fetchedPGS.Status.LastOperation != nil {
		t.Logf("LastOperation type: %s", fetchedPGS.Status.LastOperation.Type)
		t.Logf("LastOperation state: %s", fetchedPGS.Status.LastOperation.State)
		if fetchedPGS.Status.LastOperation.Description != "" {
			t.Logf("LastOperation description: %v", fetchedPGS.Status.LastOperation.Description)
		}
		t.Logf("LastOperation lastUpdateTime: %v", fetchedPGS.Status.LastOperation.LastUpdateTime)
	}

	// Debug LastErrors
	if len(fetchedPGS.Status.LastErrors) > 0 {
		t.Logf("Found %d LastErrors:", len(fetchedPGS.Status.LastErrors))
		for i, lastError := range fetchedPGS.Status.LastErrors {
			t.Logf("Error %d: code=%s, description=%s, observedAt=%v", i, lastError.Code, lastError.Description, lastError.ObservedAt)
		}

	} else {
		t.Logf("No LastErrors found")
	}

	// Wait for PCSG creation using Eventually with better polling
	assert.Eventually(t, func() bool {
		pcsgList := &grovecorev1alpha1.PodCliqueScalingGroupList{}
		err := env.Client.List(env.Ctx, pcsgList, client.InNamespace("test-ns"))
		if err != nil {
			t.Logf("Error listing PCSGs: %v", err)
			return false
		}
		t.Logf("Found %d PCSGs", len(pcsgList.Items))

		// Also check PGS status for any updates
		currentPGS := &grovecorev1alpha1.PodGangSet{}
		if err := env.Client.Get(env.Ctx, client.ObjectKey{Name: "test-pgs", Namespace: "test-ns"}, currentPGS); err == nil {
			if currentPGS.Status.LastOperation != nil {
				t.Logf("Current LastOperation state: %s", currentPGS.Status.LastOperation.State)
			}
			if len(currentPGS.Status.LastErrors) > 0 {
				t.Logf("Current LastErrors count: %d", len(currentPGS.Status.LastErrors))
				for i, lastError := range currentPGS.Status.LastErrors {
					t.Logf("Current Error %d: %s", i, lastError.Description)
				}
			}
		}

		return len(pcsgList.Items) == 1
	}, 15*time.Second, 500*time.Millisecond, "PCSG should be created")

	// Wait for PCLQ creation using Eventually with better polling
	assert.Eventually(t, func() bool {
		pclqList := &grovecorev1alpha1.PodCliqueList{}
		err := env.Client.List(env.Ctx, pclqList, client.InNamespace("test-ns"))
		if err != nil {
			t.Logf("Error listing PCLQs: %v", err)
			return false
		}
		t.Logf("Found %d PCLQs", len(pclqList.Items))
		return len(pclqList.Items) == 2
	}, 20*time.Second, 500*time.Millisecond, "Both PCLQs should be created")

	// Verify final state
	pcsgList := &grovecorev1alpha1.PodCliqueScalingGroupList{}
	err = env.Client.List(env.Ctx, pcsgList, client.InNamespace("test-ns"))
	require.NoError(t, err)
	require.Len(t, pcsgList.Items, 1)

	pclqList := &grovecorev1alpha1.PodCliqueList{}
	err = env.Client.List(env.Ctx, pclqList, client.InNamespace("test-ns"))
	require.NoError(t, err)
	require.Len(t, pclqList.Items, 2)

	// Verify ownership and basic properties
	pcsg := pcsgList.Items[0]
	assert.Equal(t, "test-pgs", pcsg.Labels["grove.io/podgangset-name"])
	assert.Equal(t, string(pgs.UID), string(pcsg.GetOwnerReferences()[0].UID))

	for _, pclq := range pclqList.Items {
		assert.Equal(t, "test-pgs", pclq.Labels["grove.io/podgangset-name"])
		assert.Equal(t, string(pcsg.UID), string(pclq.GetOwnerReferences()[0].UID))
	}
}
