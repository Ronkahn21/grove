package indexer

import (
	"strconv"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Test utilities
func createTestPod(name, hostname string, uid string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Spec: corev1.PodSpec{
			Hostname: hostname,
		},
	}
}

func createTestPodsWithIndices(indices []int) ([]*corev1.Pod, map[string]int) {
	pods := make([]*corev1.Pod, len(indices))
	podIndexMap := make(map[string]int)
	for i, idx := range indices {
		name := "pod-" + string(rune('a'+i))
		uid := "uid-" + string(rune('a'+i))
		pods[i] = createTestPod(
			name,
			"test-clique-"+strconv.Itoa(idx),
			uid,
		)
		podIndexMap[name] = idx
	}
	return pods, podIndexMap
}

func assertBidirectionalConsistency(t *testing.T, biMap *bidirectionalMap) {
	// Verify that both maps are consistent
	for podName, index := range biMap.podToIndex {
		mappedPod, exists := biMap.indexToPod[index]
		assert.True(t, exists, "Index %d missing from indexToPod map", index)
		assert.Equal(t, podName, mappedPod, "Inconsistent mapping for index %d", index)
	}

	for index, podName := range biMap.indexToPod {
		mappedIndex, exists := biMap.podToIndex[podName]
		assert.True(t, exists, "Pod %s missing from podToIndex map", podName)
		assert.Equal(t, index, mappedIndex, "Inconsistent mapping for pod %s", podName)
	}
}

// ==================== bidirectionalMap Tests ====================

func TestNewBidirectionalMap(t *testing.T) {
	biMap := newBidirectionalMap(3)

	assert.NotNil(t, biMap)
	assert.Equal(t, 3, biMap.replicaSize)
	assert.Empty(t, biMap.podToIndex)
	assert.Empty(t, biMap.indexToPod)
	assertBidirectionalConsistency(t, biMap)
}

func TestBidirectionalMap_AssignIndex(t *testing.T) {
	biMap := newBidirectionalMap(5)
	name1 := "pod-1"
	name2 := "pod-2"

	// Assign new index to new pod
	biMap.assignIndex(name1, 0)
	index, exists := biMap.getPodIndex(name1)
	assert.True(t, exists)
	assert.Equal(t, 0, index)
	assert.True(t, biMap.isIndexUsed(0))
	assertBidirectionalConsistency(t, biMap)

	// Assign different pod to same index (should replace)
	biMap.assignIndex(name2, 0)
	_, exists = biMap.getPodIndex(name1)
	assert.False(t, exists, "Previous pod should be removed")
	index, exists = biMap.getPodIndex(name2)
	assert.True(t, exists)
	assert.Equal(t, 0, index)
	assertBidirectionalConsistency(t, biMap)

	// Reassign existing pod to different index
	biMap.assignIndex(name2, 2)
	assert.False(t, biMap.isIndexUsed(0), "Previous index should be free")
	index, exists = biMap.getPodIndex(name2)
	assert.True(t, exists)
	assert.Equal(t, 2, index)
	assert.True(t, biMap.isIndexUsed(2))
	assertBidirectionalConsistency(t, biMap)
}

func TestBidirectionalMap_GetPodIndex(t *testing.T) {
	biMap := newBidirectionalMap(3)
	name := "test-pod"

	// Non-existent pod
	_, exists := biMap.getPodIndex(name)
	assert.False(t, exists)

	// Existing pod
	biMap.assignIndex(name, 1)
	index, exists := biMap.getPodIndex(name)
	assert.True(t, exists)
	assert.Equal(t, 1, index)
}

func TestBidirectionalMap_IsIndexUsed(t *testing.T) {
	biMap := newBidirectionalMap(3)
	name := "test-pod"

	// Unused index
	assert.False(t, biMap.isIndexUsed(0))
	assert.False(t, biMap.isIndexUsed(1))

	// Used index
	biMap.assignIndex(name, 1)
	assert.False(t, biMap.isIndexUsed(0))
	assert.True(t, biMap.isIndexUsed(1))
	assert.False(t, biMap.isIndexUsed(2))
}

func TestBidirectionalMap_GetAvailableIndex_HoleFilling(t *testing.T) {
	tests := []struct {
		name          string
		replicaSize   int
		usedIndices   []int
		expectedIndex int
		description   string
	}{
		{
			name:          "empty map",
			replicaSize:   3,
			usedIndices:   []int{},
			expectedIndex: 0,
			description:   "should return 0 for empty map",
		},
		{
			name:          "hole at beginning",
			replicaSize:   5,
			usedIndices:   []int{1, 2, 3},
			expectedIndex: 0,
			description:   "should fill hole at index 0",
		},
		{
			name:          "hole in middle",
			replicaSize:   5,
			usedIndices:   []int{0, 2, 4},
			expectedIndex: 1,
			description:   "should fill first hole at index 1",
		},
		{
			name:          "multiple holes",
			replicaSize:   6,
			usedIndices:   []int{1, 3, 5},
			expectedIndex: 0,
			description:   "should fill lowest hole at index 0",
		},
		{
			name:          "hole at end of replica range",
			replicaSize:   4,
			usedIndices:   []int{0, 1, 3},
			expectedIndex: 2,
			description:   "should fill hole at index 2",
		},
		{
			name:          "mix of in-range and out-of-range",
			replicaSize:   3,
			usedIndices:   []int{0, 2, 3, 5},
			expectedIndex: 1,
			description:   "should fill in-range hole first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biMap := newBidirectionalMap(tt.replicaSize)

			// Assign used indices
			for i, index := range tt.usedIndices {
				name := "pod-" + string(rune('a'+i))
				biMap.assignIndex(name, index)
			}

			actualIndex, err := biMap.getAvailableIndex()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedIndex, actualIndex, tt.description)
			assertBidirectionalConsistency(t, biMap)
		})
	}
}

func TestBidirectionalMap_GetAvailableIndex_NoAvailableIndex(t *testing.T) {
	biMap := newBidirectionalMap(3)

	// Use all indices within replica size
	biMap.assignIndex("pod-a", 0)
	biMap.assignIndex("pod-b", 1)
	biMap.assignIndex("pod-c", 2)

	// Should return error when no index available
	_, err := biMap.getAvailableIndex()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available index within replica size")

	assertBidirectionalConsistency(t, biMap)
}

// ==================== extractIndexFromHostname Tests ====================

func TestExtractIndexFromHostname_Valid(t *testing.T) {
	tests := []struct {
		hostname string
		expected int
	}{
		{"pod-0", 0},
		{"test-clique-5", 5},
		{"complex-name-with-dashes-42", 42},
		{"single-1", 1},
		{"prefix-123", 123},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			index, err := ExtractIndexFromHostname(tt.hostname)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, index)
		})
	}
}

func TestExtractIndexFromHostname_Invalid(t *testing.T) {
	tests := []struct {
		hostname    string
		description string
	}{
		{"", "empty hostname"},
		{"no-dash", "hostname without dash"},
		{"pod", "hostname without index"},
		{"pod-abc", "non-numeric index"},
		{"prefix-", "empty index"},
		{"prefix-12a", "partially numeric index"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			_, err := ExtractIndexFromHostname(tt.hostname)
			assert.Error(t, err, "should return error for: %s", tt.description)
		})
	}
}

// ==================== NewIndexManager Tests ====================

func TestNewIndexManager_EmptyPods(t *testing.T) {
	im := NewIndexManager(3, map[string]int{})

	assert.NotNil(t, im)
	assert.Equal(t, 3, im.biMap.replicaSize)
	assertBidirectionalConsistency(t, im.biMap)
}

func TestNewIndexManager_ValidPods(t *testing.T) {
	podIndexMap := map[string]int{
		"pod-a": 0,
		"pod-b": 2,
		"pod-c": 4,
	}

	im := NewIndexManager(5, podIndexMap)

	// Verify pods are assigned correctly
	index, exists := im.biMap.getPodIndex("pod-a")
	assert.True(t, exists)
	assert.Equal(t, 0, index)

	index, exists = im.biMap.getPodIndex("pod-b")
	assert.True(t, exists)
	assert.Equal(t, 2, index)

	index, exists = im.biMap.getPodIndex("pod-c")
	assert.True(t, exists)
	assert.Equal(t, 4, index)

	assertBidirectionalConsistency(t, im.biMap)
}

// ==================== GetIndex Tests ====================

func TestGetIndex_NewPods_HoleFilling(t *testing.T) {
	tests := []struct {
		name            string
		existingIndices []int
		replicaSize     int
		expectedIndex   int
		description     string
	}{
		{
			name:            "fill hole at beginning",
			existingIndices: []int{1, 2, 3},
			replicaSize:     5,
			expectedIndex:   0,
			description:     "should assign index 0 when it's available",
		},
		{
			name:            "fill first hole in middle",
			existingIndices: []int{0, 2, 4},
			replicaSize:     5,
			expectedIndex:   1,
			description:     "should assign index 1 (first available hole)",
		},
		{
			name:            "fill multiple holes - choose lowest",
			existingIndices: []int{1, 3, 5},
			replicaSize:     6,
			expectedIndex:   0,
			description:     "should assign index 0 (lowest available)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, podIndexMap := createTestPodsWithIndices(tt.existingIndices)
			im := NewIndexManager(tt.replicaSize, podIndexMap)

			newPod := createTestPod("new-pod", "irrelevant", "new-uid")
			actualIndex, err := im.GetIndex(newPod)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedIndex, actualIndex, tt.description)
			assertBidirectionalConsistency(t, im.biMap)
		})
	}
}

func TestGetIndex_ExistingPods(t *testing.T) {
	existingPods, podIndexMap := createTestPodsWithIndices([]int{0, 2, 4})
	im := NewIndexManager(5, podIndexMap)

	// Get index for existing pod - should return same index
	existingPod := existingPods[1] // Pod with index 2
	index, err := im.GetIndex(existingPod)
	require.NoError(t, err)
	assert.Equal(t, 2, index)

	// Call again - should be idempotent
	index2, err2 := im.GetIndex(existingPod)
	require.NoError(t, err2)
	assert.Equal(t, 2, index2)
	assert.Equal(t, index, index2, "should return same index on repeated calls")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_NilPod(t *testing.T) {
	im := NewIndexManager(3, map[string]int{})

	index, err := im.GetIndex(nil)
	assert.Error(t, err, "should return error for nil pod")
	assert.Equal(t, -1, index, "should return -1 for nil pod")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_SequentialAssignment(t *testing.T) {
	_, podIndexMap := createTestPodsWithIndices([]int{0, 3, 5})
	im := NewIndexManager(6, podIndexMap)

	// Add new pods sequentially - should fill holes within replica size
	expectedIndices := []int{1, 2, 4} // Only indices within replica size

	for i, expectedIndex := range expectedIndices {
		// Use unique UIDs to avoid conflicts
		pod := createTestPod("new-pod-"+strconv.Itoa(i), "irrelevant", "new-uid-"+strconv.Itoa(i))
		actualIndex, err := im.GetIndex(pod)
		require.NoError(t, err)
		assert.Equal(t, expectedIndex, actualIndex, "Pod %d should get index %d", i, expectedIndex)
	}

	// Next pod should fail - all replica indices used
	pod := createTestPod("overflow-pod", "irrelevant", "overflow-uid")
	_, err := im.GetIndex(pod)
	assert.Error(t, err, "should fail when all replica indices are used")
	assert.Contains(t, err.Error(), "no available index within replica size")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_NoAvailableIndex(t *testing.T) {
	// Create manager with all indices used
	podIndexMap := map[string]int{
		"pod-a": 0,
		"pod-b": 1,
		"pod-c": 2,
	}
	im := NewIndexManager(3, podIndexMap)

	// Try to add new pod when all indices are used
	newPod := createTestPod("new-pod", "irrelevant", "new-uid")
	index, err := im.GetIndex(newPod)
	assert.Error(t, err)
	assert.Equal(t, -1, index)
	assert.Contains(t, err.Error(), "no available index within replica size")
	assert.Contains(t, err.Error(), "new-pod")

	assertBidirectionalConsistency(t, im.biMap)
}

// ==================== Integration Tests ====================

func TestIndexManager_ComplexScenario(t *testing.T) {
	// Start with pods at indices [0, 2] (only use 2 of 4 available indices)
	_, podIndexMap := createTestPodsWithIndices([]int{0, 2})
	im := NewIndexManager(4, podIndexMap) // replica size = 4

	// Available indices should be: 1, 3 (within replica)

	// Add first new pod
	pod1 := createTestPod("pod1", "irrelevant", "uid1")
	index1, err := im.GetIndex(pod1)
	require.NoError(t, err)
	assert.Equal(t, 1, index1, "first new pod should get index 1")

	// Add second new pod
	pod2 := createTestPod("pod2", "irrelevant", "uid2")
	index2, err2 := im.GetIndex(pod2)
	require.NoError(t, err2)
	assert.Equal(t, 3, index2, "second new pod should get index 3")

	// Add third new pod (all in-range indices now used) - should fail
	pod3 := createTestPod("pod3", "irrelevant", "uid3")
	index3, err3 := im.GetIndex(pod3)
	assert.Error(t, err3, "third new pod should fail - no available indices")
	assert.Equal(t, -1, index3)
	assert.Contains(t, err3.Error(), "no available index within replica size")

	// Verify existing assignments are still consistent
	check1, _ := im.GetIndex(pod1)
	assert.Equal(t, 1, check1, "pod1 should still return index 1")
	check2, _ := im.GetIndex(pod2)
	assert.Equal(t, 3, check2, "pod2 should still return index 3")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestIndexManager_ReplicaSizeBoundary(t *testing.T) {
	// Create manager with replica size 2
	im := NewIndexManager(2, map[string]int{})

	// Add pods up to replica size
	pod0 := createTestPod("pod0", "irrelevant", "uid0")
	pod1 := createTestPod("pod1", "irrelevant", "uid1")

	idx0, err0 := im.GetIndex(pod0)
	require.NoError(t, err0)
	assert.Equal(t, 0, idx0)
	idx1, err1 := im.GetIndex(pod1)
	require.NoError(t, err1)
	assert.Equal(t, 1, idx1)

	// Next pod should fail - no more indices available
	pod2 := createTestPod("pod2", "irrelevant", "uid2")
	idx2, err2 := im.GetIndex(pod2)
	assert.Error(t, err2, "should fail when replica size exceeded")
	assert.Equal(t, -1, idx2)
	assert.Contains(t, err2.Error(), "no available index within replica size")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_ZeroReplicaSize(t *testing.T) {
	im := NewIndexManager(0, map[string]int{})

	pod := createTestPod("test-pod", "irrelevant", "test-uid")
	index, err := im.GetIndex(pod)
	assert.Error(t, err, "should fail with zero replica size - no available indices")
	assert.Equal(t, -1, index)
	assert.Contains(t, err.Error(), "no available index within replica size")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestBidirectionalMap_LargeIndices(t *testing.T) {
	biMap := newBidirectionalMap(2)
	name := "test-pod"

	// Assign a very large index
	biMap.assignIndex(name, 1000)
	index, exists := biMap.getPodIndex(name)
	assert.True(t, exists)
	assert.Equal(t, 1000, index)
	assert.True(t, biMap.isIndexUsed(1000))

	assertBidirectionalConsistency(t, biMap)
}

// createTestPodWithHostname creates a test pod with the specified name, uid, and hostname
func createTestPodWithHostname(name, uid, hostname string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Spec: corev1.PodSpec{
			Hostname: hostname,
		},
	}
}

// createTestLogger creates a simple logger for testing
func createTestLogger() logr.Logger {
	return log.Log.WithName("test")
}

// assertIndexManagerMappings verifies the IndexManager has the expected pod name to index mappings
func assertIndexManagerMappings(t *testing.T, im *IndexManager, expectedMappings map[string]int, expectedReplicaSize int) {
	// We can't directly access IndexManager internals, so we test via GetIndex behavior
	// Create test pods with the expected names and verify they return the expected indices
	for name, expectedIndex := range expectedMappings {
		testPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		actualIndex, err := im.GetIndex(testPod)
		require.NoError(t, err)
		assert.Equal(t, expectedIndex, actualIndex, "Pod name %s should have index %d", name, expectedIndex)
	}
}

// ==================== Happy Path Tests ====================

func TestInitIndexManager_ValidPods(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with valid hostnames
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-a", "uid-a", "test-clique-0"),
		createTestPodWithHostname("pod-b", "uid-b", "test-clique-2"),
		createTestPodWithHostname("pod-c", "uid-c", "test-clique-4"),
	}

	// Initialize IndexManager with expected size 5
	im := InitIndexManger(pods, 5, logger)

	// Verify mappings
	expectedMappings := map[string]int{
		"pod-a": 0,
		"pod-b": 2,
		"pod-c": 4,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 5)
}

func TestInitIndexManager_EmptyPods(t *testing.T) {
	logger := createTestLogger()

	// Initialize IndexManager with empty pod list
	im := InitIndexManger([]*corev1.Pod{}, 3, logger)

	// Verify no mappings exist (try to get index for new pod should get next available)
	newPod := createTestPodWithHostname("new-pod", "new-uid", "irrelevant")
	index, err := im.GetIndex(newPod)
	require.NoError(t, err)
	assert.Equal(t, 0, index, "should get first available index in empty manager")
}

func TestInitIndexManager_SparseIndices(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with non-consecutive indices
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-a", "uid-a", "test-clique-0"),
		createTestPodWithHostname("pod-b", "uid-b", "test-clique-2"),
		createTestPodWithHostname("pod-c", "uid-c", "test-clique-4"),
	}

	// Initialize IndexManager with expected size 6
	im := InitIndexManger(pods, 6, logger)

	// Verify mappings
	expectedMappings := map[string]int{
		"pod-a": 0,
		"pod-b": 2,
		"pod-c": 4,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 6)

	// Verify that holes can be filled (index 1 should be available)
	newPod := createTestPodWithHostname("new-pod", "new-uid", "irrelevant")
	index, err := im.GetIndex(newPod)
	require.NoError(t, err)
	assert.Equal(t, 1, index, "should fill hole at index 1")
}

// ==================== Parameter Edge Case Tests ====================

func TestInitIndexManager_ZeroExpectedSize(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with valid hostnames
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-a", "uid-a", "test-clique-0"),
		createTestPodWithHostname("pod-b", "uid-b", "test-clique-1"),
	}

	// Initialize IndexManager with expected size 0
	im := InitIndexManger(pods, 0, logger)

	// All pods should be considered out-of-bounds, so no mappings
	// Try to get index for existing pod - should get new index since it wasn't mapped
	newPod := createTestPodWithHostname("new-pod", "new-uid", "irrelevant")
	_, err := im.GetIndex(newPod)
	assert.Error(t, err, "should fail with zero replica size")
}

func TestInitIndexManager_LargeExpectedSize(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with valid hostnames
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-a", "uid-a", "test-clique-0"),
		createTestPodWithHostname("pod-b", "uid-b", "test-clique-1"),
	}

	// Initialize IndexManager with large expected size
	im := InitIndexManger(pods, 1000, logger)

	// Verify mappings work correctly
	expectedMappings := map[string]int{
		"pod-a": 0,
		"pod-b": 1,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 1000)
}

// ==================== Input Validation Tests ====================

func TestInitIndexManager_InvalidHostnames(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with mix of valid and invalid hostnames
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-valid", "uid-valid", "test-clique-0"),
		createTestPodWithHostname("pod-invalid1", "uid-invalid1", "invalid-hostname"),
		createTestPodWithHostname("pod-invalid2", "uid-invalid2", "pod-abc"),
		createTestPodWithHostname("pod-valid2", "uid-valid2", "test-clique-2"),
	}

	// Initialize IndexManager
	im := InitIndexManger(pods, 5, logger)

	// Only valid pods should be mapped
	expectedMappings := map[string]int{
		"pod-valid":  0,
		"pod-valid2": 2,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 5)
}

func TestInitIndexManager_EmptyHostnames(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with empty hostnames
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-empty1", "uid-empty1", ""),
		createTestPodWithHostname("pod-valid", "uid-valid", "test-clique-1"),
		createTestPodWithHostname("pod-empty2", "uid-empty2", ""),
	}

	// Initialize IndexManager
	im := InitIndexManger(pods, 3, logger)

	// Only pod with valid hostname should be mapped
	expectedMappings := map[string]int{
		"pod-valid": 1,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 3)
}

func TestInitIndexManager_OutOfBoundsIndices(t *testing.T) {
	logger := createTestLogger()

	// Create the valid pods that should be mapped during initialization
	validPod1 := createTestPodWithHostname("pod-valid", "uid-valid", "test-clique-1")
	validPod2 := createTestPodWithHostname("pod-valid2", "uid-valid2", "test-clique-2")

	// Create test pods with out-of-bounds indices
	pods := []*corev1.Pod{
		validPod1,
		createTestPodWithHostname("pod-negative", "uid-negative", "test-clique-negative"), // This will fail hostname parsing
		createTestPodWithHostname("pod-too-high", "uid-too-high", "test-clique-5"),
		validPod2,
	}

	// Initialize IndexManager with expected size 3 (indices 0,1,2 are valid)
	im := InitIndexManger(pods, 3, logger)

	// Only in-bounds pods should be mapped - test them using the original pod objects
	index1, err1 := im.GetIndex(validPod1)
	require.NoError(t, err1)
	assert.Equal(t, 1, index1, "pod-valid should have index 1")

	index2, err2 := im.GetIndex(validPod2)
	require.NoError(t, err2)
	assert.Equal(t, 2, index2, "pod-valid2 should have index 2")

	// Verify that a new pod gets the next available index (0)
	newPod := createTestPodWithHostname("new-pod", "uid-new", "irrelevant")
	newIndex, err := im.GetIndex(newPod)
	require.NoError(t, err)
	assert.Equal(t, 0, newIndex, "new pod should get next available index (0)")
}

// ==================== Edge Case Tests ====================

func TestInitIndexManager_DuplicateIndices(t *testing.T) {
	logger := createTestLogger()

	// Create test pods with duplicate indices
	pod1 := createTestPodWithHostname("pod-first", "uid-first", "test-clique-1")
	pod2 := createTestPodWithHostname("pod-second", "uid-second", "test-clique-1") // Same index
	pod3 := createTestPodWithHostname("pod-third", "uid-third", "test-clique-2")

	pods := []*corev1.Pod{pod1, pod2, pod3}

	// Initialize IndexManager
	im := InitIndexManger(pods, 5, logger)

	// The IndexManager's assignIndex method handles duplicates by "last wins"
	// So uid-second should have index 1, uid-first should be overwritten

	// Test the pod that should have won the conflict
	index2, err2 := im.GetIndex(pod2)
	require.NoError(t, err2)
	assert.Equal(t, 1, index2, "second pod should have index 1 (last wins)")

	// Test the pod that should not have been overwritten
	index3, err3 := im.GetIndex(pod3)
	require.NoError(t, err3)
	assert.Equal(t, 2, index3, "third pod should have index 2")

	// Verify that the first pod was overwritten and gets a new index
	index1, err1 := im.GetIndex(pod1)
	require.NoError(t, err1)
	assert.Equal(t, 0, index1, "first pod should get new index since it was overwritten")
}

func TestInitIndexManager_NilPods(t *testing.T) {
	logger := createTestLogger()

	// Create pod slice with nil pods
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-valid1", "uid-valid1", "test-clique-0"),
		nil, // Nil pod
		createTestPodWithHostname("pod-valid2", "uid-valid2", "test-clique-2"),
		nil, // Another nil pod
	}

	// Initialize IndexManager (should handle nil pods gracefully)
	im := InitIndexManger(pods, 5, logger)

	// Only valid pods should be mapped
	expectedMappings := map[string]int{
		"pod-valid1": 0,
		"pod-valid2": 2,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 5)
}

func TestInitIndexManager_MixedScenario(t *testing.T) {
	logger := createTestLogger()

	// Create a realistic mixed scenario
	pods := []*corev1.Pod{
		createTestPodWithHostname("pod-valid1", "uid-valid1", "test-clique-0"),
		createTestPodWithHostname("pod-empty", "uid-empty", ""),              // Empty hostname
		createTestPodWithHostname("pod-invalid", "uid-invalid", "malformed"), // Invalid hostname
		createTestPodWithHostname("pod-valid2", "uid-valid2", "test-clique-1"),
		createTestPodWithHostname("pod-oob", "uid-oob", "test-clique-10"), // Out of bounds
		nil, // Nil pod
		createTestPodWithHostname("pod-valid3", "uid-valid3", "test-clique-2"),
	}

	// Initialize IndexManager with expected size 5
	im := InitIndexManger(pods, 5, logger)

	// Only valid, in-bounds pods should be mapped
	expectedMappings := map[string]int{
		"pod-valid1": 0,
		"pod-valid2": 1,
		"pod-valid3": 2,
	}
	assertIndexManagerMappings(t, im, expectedMappings, 5)

	// Verify next available index is 3
	newPod := createTestPodWithHostname("new-pod", "new-uid", "irrelevant")
	index, err := im.GetIndex(newPod)
	require.NoError(t, err)
	assert.Equal(t, 3, index, "next available index should be 3")
}
