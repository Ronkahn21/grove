// /*
// Copyright 2025 The Grove Authors.
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

package kubernetes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testPgsName      = "test-pgs"
	testNamespace    = "test-ns"
	testResourceName = "test-resource"
)

// Test helper functions
func newTestObjectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

func newTestObjectMetaWithOwnerRefs(name, namespace string, ownerRefs ...metav1.OwnerReference) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            name,
		Namespace:       namespace,
		OwnerReferences: ownerRefs,
	}
}

func newTestObjectMetaWithDeletionTimestamp(name, namespace string, timestamp *metav1.Time) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:              name,
		Namespace:         namespace,
		DeletionTimestamp: timestamp,
	}
}

func newTestOwnerReference(name string, uid types.UID, isController bool) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "PodGangSet",
		Name:       name,
		UID:        uid,
		Controller: ptr.To(isController),
	}
}

func newTestOwnerReferenceSimple(name string, isController bool) metav1.OwnerReference {
	return newTestOwnerReference(name, uuid.NewUUID(), isController)
}

func TestGetDefaultLabelsForPodGangSetManagedResources(t *testing.T) {
	labels := GetDefaultLabelsForPodGangSetManagedResources(testPgsName)
	assert.Equal(t, labels, map[string]string{
		"app.kubernetes.io/managed-by": "grove-operator",
		"app.kubernetes.io/part-of":    testPgsName,
	})
}

func TestFilterMapOwnedResourceNames(t *testing.T) {
	testOwnerObjMeta := metav1.ObjectMeta{
		Name:      testPgsName,
		Namespace: testNamespace,
		UID:       uuid.NewUUID(),
	}
	testCases := []struct {
		description           string
		ownerObjMeta          metav1.ObjectMeta
		candidateResources    []metav1.PartialObjectMetadata
		expectedResourceNames []string
	}{
		{
			description:  "None of the resources are owned by the owner object",
			ownerObjMeta: testOwnerObjMeta,
			candidateResources: []metav1.PartialObjectMetadata{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "resource1",
						Namespace: testNamespace,
						UID:       uuid.NewUUID(),
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "PodGangSet",
								Name:       "other-pgs",
								UID:        uuid.NewUUID(),
								Controller: ptr.To(true),
							},
						},
					},
				},
			},
			expectedResourceNames: []string{},
		},
		{
			description:  "Some resources are owned by the owner object",
			ownerObjMeta: testOwnerObjMeta,
			candidateResources: []metav1.PartialObjectMetadata{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "resource1",
						Namespace: testNamespace,
						UID:       uuid.NewUUID(),
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "PodGangSet",
								Name:       testPgsName,
								UID:        testOwnerObjMeta.UID,
								Controller: ptr.To(true),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "resource2",
						Namespace: testNamespace,
						UID:       uuid.NewUUID(),
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "PodGangSet",
								Name:       "other-pgs",
								UID:        uuid.NewUUID(),
								Controller: ptr.To(true),
							},
						},
					},
				},
			},
			expectedResourceNames: []string{"resource1"},
		},
		{
			description:  "All resources are owned by the owner object",
			ownerObjMeta: testOwnerObjMeta,
			candidateResources: []metav1.PartialObjectMetadata{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "resource1",
						Namespace: testNamespace,
						UID:       uuid.NewUUID(),
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "PodGangSet",
								Name:       testPgsName,
								UID:        testOwnerObjMeta.UID,
								Controller: ptr.To(true),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "resource2",
						Namespace: testNamespace,
						UID:       uuid.NewUUID(),
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "PodGangSet",
								Name:       testPgsName,
								UID:        testOwnerObjMeta.UID,
								Controller: ptr.To(true),
							},
						},
					},
				},
			},
			expectedResourceNames: []string{"resource1", "resource2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			resourceNames := FilterMapOwnedResourceNames(tc.ownerObjMeta, tc.candidateResources)
			assert.Equal(t, tc.expectedResourceNames, resourceNames)
		})
	}
}

func TestGetFirstOwnerName(t *testing.T) {
	testCases := []struct {
		description  string
		resourceMeta metav1.ObjectMeta
		expectedName string
	}{
		{
			description: "should return first owner name when owner references exist",
			resourceMeta: newTestObjectMetaWithOwnerRefs(testResourceName, testNamespace,
				newTestOwnerReferenceSimple("first-owner", true),
				newTestOwnerReferenceSimple("second-owner", false)),
			expectedName: "first-owner",
		},
		{
			description: "should return single owner name when only one owner exists",
			resourceMeta: newTestObjectMetaWithOwnerRefs(testResourceName, testNamespace,
				newTestOwnerReferenceSimple("only-owner", true)),
			expectedName: "only-owner",
		},
		{
			description:  "should return empty string when no owner references exist",
			resourceMeta: newTestObjectMetaWithOwnerRefs(testResourceName, testNamespace),
			expectedName: "",
		},
		{
			description:  "should return empty string when owner references is nil",
			resourceMeta: newTestObjectMeta(testResourceName, testNamespace),
			expectedName: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := GetFirstOwnerName(tc.resourceMeta)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

func TestGetObjectKeyFromObjectMeta(t *testing.T) {
	testCases := []struct {
		description string
		objMeta     metav1.ObjectMeta
		expectedKey client.ObjectKey
	}{
		{
			description: "should create object key with namespace and name",
			objMeta:     newTestObjectMeta(testResourceName, testNamespace),
			expectedKey: client.ObjectKey{
				Name:      testResourceName,
				Namespace: testNamespace,
			},
		},
		{
			description: "should create object key with empty namespace for cluster-scoped resources",
			objMeta:     newTestObjectMeta("test-cluster-resource", ""),
			expectedKey: client.ObjectKey{
				Name:      "test-cluster-resource",
				Namespace: "",
			},
		},
		{
			description: "should create object key with empty name",
			objMeta:     newTestObjectMeta("", testNamespace),
			expectedKey: client.ObjectKey{
				Name:      "",
				Namespace: testNamespace,
			},
		},
		{
			description: "should create object key with both empty namespace and name",
			objMeta:     newTestObjectMeta("", ""),
			expectedKey: client.ObjectKey{
				Name:      "",
				Namespace: "",
			},
		},
		{
			description: "should create object key ignoring other metadata fields",
			objMeta: metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: testNamespace,
				Labels: map[string]string{
					"app": "test",
				},
				Annotations: map[string]string{
					"note": "test annotation",
				},
				UID:               uuid.NewUUID(),
				ResourceVersion:   "123",
				Generation:        1,
				CreationTimestamp: metav1.Now(),
			},
			expectedKey: client.ObjectKey{
				Name:      "test-resource",
				Namespace: testNamespace,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := GetObjectKeyFromObjectMeta(tc.objMeta)
			assert.Equal(t, tc.expectedKey, result)
		})
	}
}

func TestIsResourceTerminating(t *testing.T) {
	now := metav1.Now()

	testCases := []struct {
		description    string
		objMeta        metav1.ObjectMeta
		expectedResult bool
	}{
		{
			description:    "should return true when deletion timestamp is set",
			objMeta:        newTestObjectMetaWithDeletionTimestamp(testResourceName, testNamespace, &now),
			expectedResult: true,
		},
		{
			description:    "should return false when deletion timestamp is nil",
			objMeta:        newTestObjectMetaWithDeletionTimestamp(testResourceName, testNamespace, nil),
			expectedResult: false,
		},
		{
			description:    "should return false when deletion timestamp is not set",
			objMeta:        newTestObjectMeta(testResourceName, testNamespace),
			expectedResult: false,
		},
		{
			description: "should return true when deletion timestamp is set with other metadata",
			objMeta: metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: testNamespace,
				Labels: map[string]string{
					"app": "test",
				},
				Annotations: map[string]string{
					"note": "test annotation",
				},
				Finalizers: []string{
					"test.finalizer/cleanup",
				},
				DeletionTimestamp:          &now,
				DeletionGracePeriodSeconds: ptr.To(int64(30)),
			},
			expectedResult: true,
		},
		{
			description: "should return true when deletion timestamp is in the past",
			objMeta: metav1.ObjectMeta{
				Name:              "test-resource",
				Namespace:         testNamespace,
				DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			},
			expectedResult: true,
		},
		{
			description: "should return true when deletion timestamp is in the future",
			objMeta: metav1.ObjectMeta{
				Name:              "test-resource",
				Namespace:         testNamespace,
				DeletionTimestamp: &metav1.Time{Time: time.Now().Add(5 * time.Minute)},
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := IsResourceTerminating(tc.objMeta)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
