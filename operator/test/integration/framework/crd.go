package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	grovecrds "github.com/NVIDIA/grove/operator/api/core/v1alpha1/crds"
	schedulercrds "github.com/NVIDIA/grove/scheduler/api/core/v1alpha1/crds"
)

const (
	crdReadyTimeout = 2 * time.Minute
	crdReadyPoll    = 1 * time.Second
)

// installGroveCRDs installs the Grove operator CRDs
func installGroveCRDs(ctx context.Context, client client.Client) error {
	crdContents := []string{
		grovecrds.PodGangSetCRD(),
		grovecrds.PodCliqueCRD(),
		grovecrds.PodCliqueScalingGroupCRD(),
	}

	for _, crdContent := range crdContents {
		if err := installCRDFromContent(ctx, client, crdContent); err != nil {
			return fmt.Errorf("failed to install CRD: %w", err)
		}
	}

	return nil
}

// installSchedulerCRDs installs the Grove scheduler CRDs
func installSchedulerCRDs(ctx context.Context, client client.Client) error {
	crdContents := []string{
		schedulercrds.PodGangCRD(),
	}

	for _, crdContent := range crdContents {
		if err := installCRDFromContent(ctx, client, crdContent); err != nil {
			return fmt.Errorf("failed to install scheduler CRD: %w", err)
		}
	}

	return nil
}

// installCRDFromContent creates a CRD from YAML content
func installCRDFromContent(ctx context.Context, client client.Client, content string) error {
	crd, err := parseCRDFromYAML(content)
	if err != nil {
		return fmt.Errorf("failed to parse CRD: %w", err)
	}

	if err := createOrUpdateCRD(ctx, client, crd); err != nil {
		return fmt.Errorf("failed to create/update CRD %s: %w", crd.Name, err)
	}

	if err := waitForCRDReady(ctx, client, crd.Name); err != nil {
		return fmt.Errorf("CRD %s not ready: %w", crd.Name, err)
	}

	return nil
}

// parseCRDFromYAML parses a CRD from YAML content
func parseCRDFromYAML(yamlContent string) (*apiextensionsv1.CustomResourceDefinition, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}

	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(yamlContent), 4096)
	if err := decoder.Decode(crd); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	return crd, nil
}

// createOrUpdateCRD creates or updates a CRD
func createOrUpdateCRD(ctx context.Context, k8sClient client.Client, crd *apiextensionsv1.CustomResourceDefinition) error {
	existing := &apiextensionsv1.CustomResourceDefinition{}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), existing)

	if errors.IsNotFound(err) {
		// Create new CRD
		return k8sClient.Create(ctx, crd)
	} else if err != nil {
		return fmt.Errorf("failed to get existing CRD: %w", err)
	}

	// Update existing CRD
	crd.ResourceVersion = existing.ResourceVersion
	return k8sClient.Update(ctx, crd)
}

// waitForCRDReady waits for a CRD to be ready for use
func waitForCRDReady(ctx context.Context, k8sClient client.Client, crdName string) error {
	return wait.PollUntilContextTimeout(ctx, crdReadyPoll, crdReadyTimeout, false, func(ctx context.Context) (bool, error) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: crdName}, crd)

		if errors.IsNotFound(err) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		// Check if CRD is established
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established &&
				condition.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})
}

// getAllGroveCRDNames returns all Grove operator CRD names
func getAllGroveCRDNames() []string {
	return []string{
		"podgangsets.grove.io",
		"podcliques.grove.io",
		"podcliquescalinggroups.grove.io",
	}
}

// cleanupGroveCRDs removes Grove operator CRDs (for cleanup)
func cleanupGroveCRDs(ctx context.Context, client client.Client) error {
	crdNames := getAllGroveCRDNames()

	for _, name := range crdNames {
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}

		err := client.Delete(ctx, crd)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete CRD %s: %w", name, err)
		}
	}

	return nil
}
