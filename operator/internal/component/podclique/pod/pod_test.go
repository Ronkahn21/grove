/*
Copyright 2025 The Grove Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pod

import (
	"testing"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddGroveEnvironmentVariables(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	grovecorev1alpha1.AddToScheme(scheme)
	
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	eventRecorder := record.NewFakeRecorder(10)
	
	resource := &_resource{
		client:        client,
		scheme:        scheme,
		eventRecorder: eventRecorder,
	}

	tests := []struct {
		name              string
		pclq              *grovecorev1alpha1.PodClique
		expectedEnvVars   []string
		unexpectedEnvVars []string
	}{
		{
			name: "standalone PodClique",
			pclq: &grovecorev1alpha1.PodClique{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pclq",
					Namespace: "test-ns",
				},
				Spec: grovecorev1alpha1.PodCliqueSpec{
					PodSpec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "test-image",
							},
						},
					},
				},
			},
			expectedEnvVars: []string{
				envVarGrovePGSName,
				envVarGrovePGSIndex,
				envVarGrovePCLQName,
				envVarPodNamespace,
			},
			unexpectedEnvVars: []string{
				envVarGrovePCSGName,
			},
		},
		{
			name: "PCSG member PodClique",
			pclq: &grovecorev1alpha1.PodClique{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pclq",
					Namespace: "test-ns",
					Labels: map[string]string{
						grovecorev1alpha1.LabelPodCliqueScalingGroup: "test-pcsg",
					},
				},
				Spec: grovecorev1alpha1.PodCliqueSpec{
					PodSpec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "test-image",
							},
						},
					},
				},
			},
			expectedEnvVars: []string{
				envVarGrovePGSName,
				envVarGrovePGSIndex,
				envVarGrovePCLQName,
				envVarPodNamespace,
				envVarGrovePCSGName,
			},
			unexpectedEnvVars: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				Spec: tt.pclq.Spec.PodSpec,
			}

			resource.addGroveEnvironmentVariables(pod, tt.pclq)

			// Check that all containers have the expected environment variables
			for _, container := range pod.Spec.Containers {
				envVarNames := make(map[string]bool)
				for _, env := range container.Env {
					envVarNames[env.Name] = true
				}

				// Check expected environment variables are present
				for _, expectedEnv := range tt.expectedEnvVars {
					if !envVarNames[expectedEnv] {
						t.Errorf("expected environment variable %s not found in container %s", expectedEnv, container.Name)
					}
				}

				// Check unexpected environment variables are not present
				for _, unexpectedEnv := range tt.unexpectedEnvVars {
					if envVarNames[unexpectedEnv] {
						t.Errorf("unexpected environment variable %s found in container %s", unexpectedEnv, container.Name)
					}
				}

				// Verify Downward API configuration for expected variables
				for _, env := range container.Env {
					if env.ValueFrom == nil || env.ValueFrom.FieldRef == nil {
						t.Errorf("environment variable %s should use Downward API FieldRef", env.Name)
						continue
					}

					// Check specific field paths
					switch env.Name {
					case envVarGrovePGSName:
						if env.ValueFrom.FieldRef.FieldPath != "metadata.labels['app.kubernetes.io/part-of']" {
							t.Errorf("incorrect field path for %s: got %s", env.Name, env.ValueFrom.FieldRef.FieldPath)
						}
					case envVarGrovePGSIndex:
						if env.ValueFrom.FieldRef.FieldPath != "metadata.labels['grove.io/podgangset-replica-index']" {
							t.Errorf("incorrect field path for %s: got %s", env.Name, env.ValueFrom.FieldRef.FieldPath)
						}
					case envVarGrovePCLQName:
						if env.ValueFrom.FieldRef.FieldPath != "metadata.labels['grove.io/podclique']" {
							t.Errorf("incorrect field path for %s: got %s", env.Name, env.ValueFrom.FieldRef.FieldPath)
						}
					case envVarGrovePCSGName:
						if env.ValueFrom.FieldRef.FieldPath != "metadata.labels['grove.io/podcliquescalinggroup']" {
							t.Errorf("incorrect field path for %s: got %s", env.Name, env.ValueFrom.FieldRef.FieldPath)
						}
					case envVarPodNamespace:
						if env.ValueFrom.FieldRef.FieldPath != "metadata.namespace" {
							t.Errorf("incorrect field path for %s: got %s", env.Name, env.ValueFrom.FieldRef.FieldPath)
						}
					}
				}
			}
		})
	}
}