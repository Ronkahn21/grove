package framework

import (
	"fmt"
	"strings"
	"time"

	grovecrds "github.com/NVIDIA/grove/operator/api/core/v1alpha1/crds"
	schedulercrds "github.com/NVIDIA/grove/scheduler/api/core/v1alpha1/crds"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	crdReadyTimeout = 2 * time.Minute
	crdReadyPoll    = 1 * time.Second
)

func getOperatorCRDs() ([]*apiextensionsv1.CustomResourceDefinition, error) {
	crdContents := []string{
		grovecrds.PodGangSetCRD(),
		grovecrds.PodCliqueCRD(),
		grovecrds.PodCliqueScalingGroupCRD(),
	}

	var crds []*apiextensionsv1.CustomResourceDefinition
	for _, crdContent := range crdContents {
		crd, err := parseCRDFromYAML(crdContent)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CRD: %w", err)
		}
		crds = append(crds, crd)
	}

	return crds, nil
}

func getSchedulerCRDs() ([]*apiextensionsv1.CustomResourceDefinition, error) {
	crdContents := []string{
		schedulercrds.PodGangCRD(),
	}

	var crds []*apiextensionsv1.CustomResourceDefinition
	for _, crdContent := range crdContents {
		crd, err := parseCRDFromYAML(crdContent)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scheduler CRD: %w", err)
		}
		crds = append(crds, crd)
	}

	return crds, nil
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
