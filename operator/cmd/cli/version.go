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
	"fmt"
	"runtime/debug"

	apicommonconstants "github.com/ai-dynamo/grove/operator/api/common/constants"
)

// GetBuildInfo gets the operator build information.
// If verbose is true, it returns detailed build information else it will only return the application version.
func GetBuildInfo(verbose bool) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Printf("%s: binary build info not embedded\n", apicommonconstants.OperatorName)
		return ""
	}
	if !verbose {
		return fmt.Sprintf("%s version: %s\n", apicommonconstants.OperatorName, info.Main.Version)
	}
	return getVerboseBuildInfo(info)
}

func getVerboseBuildInfo(info *debug.BuildInfo) string {
	// Git information
	gitVersion := getSetting(info, "vcs.version")
	commitTime := getSetting(info, "vcs.time")
	dirty := getSetting(info, "vcs.modified")
	// OS and Arch
	os := getSetting(info, "GOOS")
	arch := getSetting(info, "GOARCH")

	return fmt.Sprintf(`
%s Build Information:
  Version:    %s
  Go version: %s
  OS/Arch:    %s/%s
  Git Information:
   Revision (commit hash): %s
   Commit Time: %s
   Modified (dirty): %s
`, apicommonconstants.OperatorName, info.Main.Version, info.GoVersion, os, arch, gitVersion, commitTime, dirty)
}

func getSetting(info *debug.BuildInfo, key string) string {
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return fmt.Sprintf("<unknown %s>", key)
}
