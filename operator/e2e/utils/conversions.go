//go:build e2e

package utils

import (
	"encoding/json"
)

// ConvertUnstructuredToTyped converts an unstructured map to a typed object
func ConvertUnstructuredToTyped(u map[string]interface{}, typed interface{}) error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, typed)
}
