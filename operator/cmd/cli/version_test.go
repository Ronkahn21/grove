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

package cli

import (
	"runtime/debug"
	"testing"

	apicommonconstants "github.com/ai-dynamo/grove/operator/api/common/constants"

	"github.com/stretchr/testify/assert"
)

// TestGetSetting tests the getSetting helper function.
func TestGetSetting(t *testing.T) {
	tests := []struct {
		name     string
		info     *debug.BuildInfo
		key      string
		expected string
	}{
		{
			name: "setting exists",
			info: &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.version", Value: "abc123"},
					{Key: "vcs.time", Value: "2025-12-28T10:00:00Z"},
					{Key: "GOOS", Value: "linux"},
				},
			},
			key:      "vcs.version",
			expected: "abc123",
		},
		{
			name: "setting does not exist",
			info: &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.version", Value: "abc123"},
					{Key: "GOOS", Value: "linux"},
				},
			},
			key:      "nonexistent.key",
			expected: "<unknown nonexistent.key>",
		},
		{
			name: "empty key",
			info: &debug.BuildInfo{
				Settings: []debug.BuildSetting{},
			},
			key:      "vcs.version",
			expected: "<unknown vcs.version>",
		},
		{
			name: "settings is nil",
			info: &debug.BuildInfo{
				Settings: nil,
			},
			key:      "vcs.version",
			expected: "<unknown vcs.version>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSetting(tt.info, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetVerboseBuildInfo tests the getVerboseBuildInfo helper function.
func TestGetVerboseBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		info             *debug.BuildInfo
		expectedContains []string
		expectedNotEmpty bool
	}{
		{
			name: "build information is complete",
			info: &debug.BuildInfo{
				Main: debug.Module{
					Version: "v1.0.0",
				},
				GoVersion: "go1.21.5",
				Settings: []debug.BuildSetting{
					{Key: "vcs.version", Value: "abc123def456"},
					{Key: "vcs.time", Value: "2025-12-28T10:00:00Z"},
					{Key: "vcs.modified", Value: "false"},
					{Key: "GOOS", Value: "linux"},
					{Key: "GOARCH", Value: "amd64"},
				},
			},
			expectedContains: []string{
				apicommonconstants.OperatorName + " Build Information:",
				"Version:    v1.0.0",
				"Go version: go1.21.5",
				"OS/Arch:    linux/amd64",
				"Revision (commit hash): abc123def456",
				"Commit Time: 2025-12-28T10:00:00Z",
				"Modified (dirty): false",
			},
			expectedNotEmpty: true,
		},
		{
			name: "build information is missing some settings",
			info: &debug.BuildInfo{
				Main: debug.Module{
					Version: "v0.1.0-dev",
				},
				GoVersion: "go1.22.0",
				Settings: []debug.BuildSetting{
					{Key: "GOOS", Value: "darwin"},
					{Key: "GOARCH", Value: "arm64"},
				},
			},
			expectedContains: []string{
				apicommonconstants.OperatorName + " Build Information:",
				"Version:    v0.1.0-dev",
				"Go version: go1.22.0",
				"OS/Arch:    darwin/arm64",
				"<unknown vcs.version>",
				"<unknown vcs.time>",
				"<unknown vcs.modified>",
			},
			expectedNotEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getVerboseBuildInfo(tt.info)

			if tt.expectedNotEmpty {
				assert.NotEmpty(t, result)
			}

			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected,
					"result should contain '%s'", expected)
			}
		})
	}
}
