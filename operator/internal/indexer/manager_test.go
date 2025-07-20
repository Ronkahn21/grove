package indexer

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func createTestPodsWithIndices(indices []int) []*corev1.Pod {
	pods := make([]*corev1.Pod, len(indices))
	for i, idx := range indices {
		pods[i] = createTestPod(
			"pod-"+string(rune('a'+i)),
			"test-clique-"+strconv.Itoa(idx),
			"uid-"+string(rune('a'+i)),
		)
	}
	return pods
}

func assertBidirectionalConsistency(t *testing.T, biMap *bidirectionalMap) {
	// Verify that both maps are consistent
	for podUID, index := range biMap.podToIndex {
		mappedPod, exists := biMap.indexToPod[index]
		assert.True(t, exists, "Index %d missing from indexToPod map", index)
		assert.Equal(t, podUID, mappedPod, "Inconsistent mapping for index %d", index)
	}

	for index, podUID := range biMap.indexToPod {
		mappedIndex, exists := biMap.podToIndex[podUID]
		assert.True(t, exists, "Pod %s missing from podToIndex map", podUID)
		assert.Equal(t, index, mappedIndex, "Inconsistent mapping for pod %s", podUID)
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
	uid1 := types.UID("pod-1")
	uid2 := types.UID("pod-2")

	// Assign new index to new pod
	biMap.assignIndex(uid1, 0)
	index, exists := biMap.getPodIndex(uid1)
	assert.True(t, exists)
	assert.Equal(t, 0, index)
	assert.True(t, biMap.isIndexUsed(0))
	assertBidirectionalConsistency(t, biMap)

	// Assign different pod to same index (should replace)
	biMap.assignIndex(uid2, 0)
	_, exists = biMap.getPodIndex(uid1)
	assert.False(t, exists, "Previous pod should be removed")
	index, exists = biMap.getPodIndex(uid2)
	assert.True(t, exists)
	assert.Equal(t, 0, index)
	assertBidirectionalConsistency(t, biMap)

	// Reassign existing pod to different index
	biMap.assignIndex(uid2, 2)
	assert.False(t, biMap.isIndexUsed(0), "Previous index should be free")
	index, exists = biMap.getPodIndex(uid2)
	assert.True(t, exists)
	assert.Equal(t, 2, index)
	assert.True(t, biMap.isIndexUsed(2))
	assertBidirectionalConsistency(t, biMap)
}

func TestBidirectionalMap_GetPodIndex(t *testing.T) {
	biMap := newBidirectionalMap(3)
	uid := types.UID("test-pod")

	// Non-existent pod
	_, exists := biMap.getPodIndex(uid)
	assert.False(t, exists)

	// Existing pod
	biMap.assignIndex(uid, 1)
	index, exists := biMap.getPodIndex(uid)
	assert.True(t, exists)
	assert.Equal(t, 1, index)
}

func TestBidirectionalMap_IsIndexUsed(t *testing.T) {
	biMap := newBidirectionalMap(3)
	uid := types.UID("test-pod")

	// Unused index
	assert.False(t, biMap.isIndexUsed(0))
	assert.False(t, biMap.isIndexUsed(1))

	// Used index
	biMap.assignIndex(uid, 1)
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
			name:          "consecutive indices",
			replicaSize:   3,
			usedIndices:   []int{0, 1, 2},
			expectedIndex: 3,
			description:   "should return first out-of-range index",
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
			name:          "all replica indices used",
			replicaSize:   3,
			usedIndices:   []int{0, 1, 2},
			expectedIndex: 3,
			description:   "should assign out-of-range index",
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
				uid := types.UID("pod-" + string(rune('a'+i)))
				biMap.assignIndex(uid, index)
			}

			actualIndex := biMap.getAvailableIndex()
			assert.Equal(t, tt.expectedIndex, actualIndex, tt.description)
			assertBidirectionalConsistency(t, biMap)
		})
	}
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
			index, err := extractIndexFromHostname(tt.hostname)
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
			_, err := extractIndexFromHostname(tt.hostname)
			assert.Error(t, err, "should return error for: %s", tt.description)
		})
	}
}

// ==================== NewIndexManager Tests ====================

func TestNewIndexManager_EmptyPods(t *testing.T) {
	im, initErrors := NewIndexManager(3, []*corev1.Pod{})

	assert.NotNil(t, im)
	assert.Empty(t, initErrors)
	assert.Equal(t, 3, im.biMap.replicaSize)
	assertBidirectionalConsistency(t, im.biMap)
}

func TestNewIndexManager_ValidPods(t *testing.T) {
	pods := []*corev1.Pod{
		createTestPod("pod-a", "test-clique-0", "uid-a"),
		createTestPod("pod-b", "test-clique-2", "uid-b"),
		createTestPod("pod-c", "test-clique-4", "uid-c"),
	}

	im, initErrors := NewIndexManager(5, pods)

	assert.Empty(t, initErrors)
	// Verify pods are assigned correctly
	index, exists := im.biMap.getPodIndex("uid-a")
	assert.True(t, exists)
	assert.Equal(t, 0, index)

	index, exists = im.biMap.getPodIndex("uid-b")
	assert.True(t, exists)
	assert.Equal(t, 2, index)

	index, exists = im.biMap.getPodIndex("uid-c")
	assert.True(t, exists)
	assert.Equal(t, 4, index)

	assertBidirectionalConsistency(t, im.biMap)
}

func TestNewIndexManager_DuplicateIndices(t *testing.T) {
	pods := []*corev1.Pod{
		createTestPod("pod-a", "test-clique-0", "uid-a"),
		createTestPod("pod-b", "test-clique-0", "uid-b"), // Duplicate index
		createTestPod("pod-c", "test-clique-1", "uid-c"),
	}

	im, initErrors := NewIndexManager(3, pods)

	// Should have one error for duplicate index
	assert.Len(t, initErrors, 1)
	assert.Contains(t, initErrors[0].Error(), "pod-b")
	assert.Contains(t, initErrors[0].Error(), "duplicate index 0")

	// First pod should keep the index
	index, exists := im.biMap.getPodIndex("uid-a")
	assert.True(t, exists)
	assert.Equal(t, 0, index)

	// Second pod with duplicate index should be ignored
	_, exists = im.biMap.getPodIndex("uid-b")
	assert.False(t, exists)

	// Third pod should be assigned normally
	index, exists = im.biMap.getPodIndex("uid-c")
	assert.True(t, exists)
	assert.Equal(t, 1, index)

	assertBidirectionalConsistency(t, im.biMap)
}

func TestNewIndexManager_InvalidHostnames(t *testing.T) {
	pods := []*corev1.Pod{
		createTestPod("pod-a", "test-clique-0", "uid-a"), // Valid
		createTestPod("pod-b", "invalid", "uid-b"),       // Invalid hostname
		createTestPod("pod-c", "", "uid-c"),              // Empty hostname
		createTestPod("pod-d", "test-clique-2", "uid-d"), // Valid
	}

	im, initErrors := NewIndexManager(5, pods)
	assert.Len(t, initErrors, 2) // Should have 2 errors for invalid hostnames

	// Valid pods should be assigned
	index, exists := im.biMap.getPodIndex("uid-a")
	assert.True(t, exists)
	assert.Equal(t, 0, index)

	index, exists = im.biMap.getPodIndex("uid-d")
	assert.True(t, exists)
	assert.Equal(t, 2, index)

	// Invalid pods should be ignored
	_, exists = im.biMap.getPodIndex("uid-b")
	assert.False(t, exists)

	_, exists = im.biMap.getPodIndex("uid-c")
	assert.False(t, exists)

	assertBidirectionalConsistency(t, im.biMap)
}

func TestNewIndexManager_NilPods(t *testing.T) {
	pods := []*corev1.Pod{
		createTestPod("pod-a", "test-clique-0", "uid-a"),
		nil, // Nil pod should be skipped
		createTestPod("pod-c", "test-clique-2", "uid-c"),
	}

	im, initErrors := NewIndexManager(5, pods)
	assert.Empty(t, initErrors) // Nil pods should be skipped, no errors

	// Valid pods should be assigned
	index, exists := im.biMap.getPodIndex("uid-a")
	assert.True(t, exists)
	assert.Equal(t, 0, index)

	index, exists = im.biMap.getPodIndex("uid-c")
	assert.True(t, exists)
	assert.Equal(t, 2, index)

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
		{
			name:            "all replica indices used",
			existingIndices: []int{0, 1, 2},
			replicaSize:     3,
			expectedIndex:   3,
			description:     "should assign out-of-range index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingPods := createTestPodsWithIndices(tt.existingIndices)
			im, initErrors := NewIndexManager(tt.replicaSize, existingPods)
			assert.Empty(t, initErrors)

			newPod := createTestPod("new-pod", "irrelevant", "new-uid")
			actualIndex, err := im.GetIndex(newPod)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedIndex, actualIndex, tt.description)
			assertBidirectionalConsistency(t, im.biMap)
		})
	}
}

func TestGetIndex_ExistingPods(t *testing.T) {
	existingPods := createTestPodsWithIndices([]int{0, 2, 4})
	im, initErrors := NewIndexManager(5, existingPods)
	assert.Empty(t, initErrors)

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
	im, initErrors := NewIndexManager(3, []*corev1.Pod{})
	assert.Empty(t, initErrors)

	index, err := im.GetIndex(nil)
	assert.Error(t, err, "should return error for nil pod")
	assert.Equal(t, -1, index, "should return -1 for nil pod")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_SequentialAssignment(t *testing.T) {
	existingPods := createTestPodsWithIndices([]int{0, 3, 5})
	im, initErrors := NewIndexManager(6, existingPods)
	assert.Empty(t, initErrors)

	// Add new pods sequentially
	expectedIndices := []int{1, 2, 4, 6, 7} // Should fill holes then continue

	for i, expectedIndex := range expectedIndices {
		// Use unique UIDs to avoid conflicts
		pod := createTestPod("new-pod-"+strconv.Itoa(i), "irrelevant", "new-uid-"+strconv.Itoa(i))
		actualIndex, err := im.GetIndex(pod)
		require.NoError(t, err)
		assert.Equal(t, expectedIndex, actualIndex, "Pod %d should get index %d", i, expectedIndex)
	}

	assertBidirectionalConsistency(t, im.biMap)
}

// ==================== Integration Tests ====================

func TestIndexManager_ComplexScenario(t *testing.T) {
	// Start with pods at indices [0, 3, 5]
	initialPods := createTestPodsWithIndices([]int{0, 3, 5})
	im, initErrors := NewIndexManager(4, initialPods) // replica size = 4
	assert.Empty(t, initErrors)

	// Available indices should be: 1, 2 (within replica), then 4, 6, 7...

	// Add first new pod
	pod1 := createTestPod("pod1", "irrelevant", "uid1")
	index1, err := im.GetIndex(pod1)
	require.NoError(t, err)
	assert.Equal(t, 1, index1, "first new pod should get index 1")

	// Add second new pod
	pod2 := createTestPod("pod2", "irrelevant", "uid2")
	index2, err2 := im.GetIndex(pod2)
	require.NoError(t, err2)
	assert.Equal(t, 2, index2, "second new pod should get index 2")

	// Add third new pod (all in-range indices now used)
	pod3 := createTestPod("pod3", "irrelevant", "uid3")
	index3, err3 := im.GetIndex(pod3)
	require.NoError(t, err3)
	assert.Equal(t, 4, index3, "third new pod should get index 4 (out of range)")

	// Verify all assignments are consistent
	check1, _ := im.GetIndex(pod1)
	assert.Equal(t, 1, check1, "pod1 should still return index 1")
	check2, _ := im.GetIndex(pod2)
	assert.Equal(t, 2, check2, "pod2 should still return index 2")
	check3, _ := im.GetIndex(pod3)
	assert.Equal(t, 4, check3, "pod3 should still return index 4")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestIndexManager_ReplicaSizeBoundary(t *testing.T) {
	// Create manager with replica size 2
	im, initErrors := NewIndexManager(2, []*corev1.Pod{})
	assert.Empty(t, initErrors)

	// Add pods up to replica size
	pod0 := createTestPod("pod0", "irrelevant", "uid0")
	pod1 := createTestPod("pod1", "irrelevant", "uid1")

	idx0, _ := im.GetIndex(pod0)
	assert.Equal(t, 0, idx0)
	idx1, _ := im.GetIndex(pod1)
	assert.Equal(t, 1, idx1)

	// Next pod should get out-of-range index
	pod2 := createTestPod("pod2", "irrelevant", "uid2")
	idx2, _ := im.GetIndex(pod2)
	assert.Equal(t, 2, idx2)

	pod3 := createTestPod("pod3", "irrelevant", "uid3")
	idx3, _ := im.GetIndex(pod3)
	assert.Equal(t, 3, idx3)

	assertBidirectionalConsistency(t, im.biMap)
}

func TestGetIndex_ZeroReplicaSize(t *testing.T) {
	im, initErrors := NewIndexManager(0, []*corev1.Pod{})
	assert.Empty(t, initErrors)

	pod := createTestPod("test-pod", "irrelevant", "test-uid")
	index, err := im.GetIndex(pod)
	require.NoError(t, err)
	assert.Equal(t, 0, index, "with zero replica size, should assign index 0")

	assertBidirectionalConsistency(t, im.biMap)
}

func TestBidirectionalMap_LargeIndices(t *testing.T) {
	biMap := newBidirectionalMap(2)
	uid := types.UID("test-pod")

	// Assign a very large index
	biMap.assignIndex(uid, 1000)
	index, exists := biMap.getPodIndex(uid)
	assert.True(t, exists)
	assert.Equal(t, 1000, index)
	assert.True(t, biMap.isIndexUsed(1000))

	assertBidirectionalConsistency(t, biMap)
}
