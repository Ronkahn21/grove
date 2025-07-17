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
	"github.com/stretchr/testify/assert"
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
				envVarGrovePCLQIndex,
				envVarGrovePCSGIndex,
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
				envVarGrovePCLQIndex,
				envVarGrovePCSGIndex,
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

func TestAddGroveEnvironmentVariables_NoDuplicates(t *testing.T) {
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
		name           string
		pclq           *grovecorev1alpha1.PodClique
		existingEnvVars []corev1.EnvVar
		expectedEnvVars []string
		shouldReplace   map[string]string  // env var name -> expected value
		shouldPreserve  []string          // env var names that should be preserved
	}{
		{
			name: "Container with existing Grove env vars - should replace",
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
								Env: []corev1.EnvVar{
									{Name: "GROVE_PGS_NAME", Value: "old-pgs-name"},
									{Name: "GROVE_PGS_INDEX", Value: "old-index"},
								},
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
			shouldReplace: map[string]string{
				"GROVE_PGS_NAME":  "metadata.labels['app.kubernetes.io/part-of']",
				"GROVE_PGS_INDEX": "metadata.labels['grove.io/podgangset-replica-index']",
			},
		},
		{
			name: "Container with user env vars - should preserve",
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
								Env: []corev1.EnvVar{
									{Name: "USER_VAR", Value: "user-value"},
									{Name: "CUSTOM_CONFIG", Value: "custom-value"},
								},
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
			shouldPreserve: []string{"USER_VAR", "CUSTOM_CONFIG"},
		},
		{
			name: "Container with mixed env vars - should replace Grove, preserve user",
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
								Env: []corev1.EnvVar{
									{Name: "USER_VAR", Value: "user-value"},
									{Name: "GROVE_PGS_NAME", Value: "old-pgs-name"},
									{Name: "CUSTOM_CONFIG", Value: "custom-value"},
								},
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
			shouldReplace: map[string]string{
				"GROVE_PGS_NAME": "metadata.labels['app.kubernetes.io/part-of']",
			},
			shouldPreserve: []string{"USER_VAR", "CUSTOM_CONFIG"},
		},
		{
			name: "PCSG PodClique with existing env vars",
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
								Env: []corev1.EnvVar{
									{Name: "GROVE_PCSG_NAME", Value: "old-pcsg-name"},
									{Name: "USER_VAR", Value: "user-value"},
								},
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
				envVarGrovePCLQIndex,
				envVarGrovePCSGIndex,
			},
			shouldReplace: map[string]string{
				"GROVE_PCSG_NAME": "metadata.labels['grove.io/podcliquescalinggroup']",
			},
			shouldPreserve: []string{"USER_VAR"},
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
				envVarNames := make(map[string]corev1.EnvVar)
				for _, env := range container.Env {
					envVarNames[env.Name] = env
				}

				// Check expected environment variables are present
				for _, expectedEnv := range tt.expectedEnvVars {
					if _, found := envVarNames[expectedEnv]; !found {
						t.Errorf("expected environment variable %s not found in container %s", expectedEnv, container.Name)
					}
				}

				// Check that Grove environment variables were replaced with correct Downward API values
				if tt.shouldReplace != nil {
					for envName, expectedFieldPath := range tt.shouldReplace {
						if envVar, found := envVarNames[envName]; found {
							if envVar.ValueFrom == nil || envVar.ValueFrom.FieldRef == nil {
								t.Errorf("environment variable %s should use Downward API FieldRef", envName)
							} else if envVar.ValueFrom.FieldRef.FieldPath != expectedFieldPath {
								t.Errorf("environment variable %s has wrong field path: got %s, expected %s", 
									envName, envVar.ValueFrom.FieldRef.FieldPath, expectedFieldPath)
							}
						}
					}
				}

				// Check that user environment variables were preserved
				if tt.shouldPreserve != nil {
					for _, preserveEnv := range tt.shouldPreserve {
						if envVar, found := envVarNames[preserveEnv]; found {
							if envVar.Value == "" {
								t.Errorf("preserved environment variable %s should have its original value", preserveEnv)
							}
						} else {
							t.Errorf("expected preserved environment variable %s not found in container %s", preserveEnv, container.Name)
						}
					}
				}

				// Check that no environment variable appears more than once
				envVarCounts := make(map[string]int)
				for _, env := range container.Env {
					envVarCounts[env.Name]++
				}
				for envName, count := range envVarCounts {
					if count > 1 {
						t.Errorf("environment variable %s appears %d times (should be 1)", envName, count)
					}
				}
			}
		})
	}
}

func TestAddGroveEnvironmentVariables_Idempotent(t *testing.T) {
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

	pclq := &grovecorev1alpha1.PodClique{
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
						Env: []corev1.EnvVar{
							{Name: "USER_VAR", Value: "user-value"},
						},
					},
				},
			},
		},
	}

	pod := &corev1.Pod{
		Spec: pclq.Spec.PodSpec,
	}

	// Call addGroveEnvironmentVariables multiple times
	resource.addGroveEnvironmentVariables(pod, pclq)
	envVarsAfterFirst := make([]corev1.EnvVar, len(pod.Spec.Containers[0].Env))
	copy(envVarsAfterFirst, pod.Spec.Containers[0].Env)

	resource.addGroveEnvironmentVariables(pod, pclq)
	envVarsAfterSecond := pod.Spec.Containers[0].Env

	resource.addGroveEnvironmentVariables(pod, pclq)
	envVarsAfterThird := pod.Spec.Containers[0].Env

	// All calls should produce the same result
	assert.Equal(t, envVarsAfterFirst, envVarsAfterSecond, "second call should produce same result")
	assert.Equal(t, envVarsAfterFirst, envVarsAfterThird, "third call should produce same result")

	// Check that no environment variable appears more than once
	envVarCounts := make(map[string]int)
	for _, env := range envVarsAfterThird {
		envVarCounts[env.Name]++
	}
	for envName, count := range envVarCounts {
		if count > 1 {
			t.Errorf("environment variable %s appears %d times after multiple calls (should be 1)", envName, count)
		}
	}
}