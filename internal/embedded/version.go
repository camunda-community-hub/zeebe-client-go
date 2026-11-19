// Copyright © 2018 Camunda Services GmbH (info@camunda.com)
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

package embedded

import (
	"embed"
	"runtime/debug"
	"strings"
)

// ModulePath is this client's Go module path.
const ModulePath = "github.com/camunda-community-hub/zeebe-client-go/v8"

var (
	//go:embed data
	data embed.FS
	// Version is the client version reported in the user agent. It is the version of
	// this module in the running binary's build info (the real release or
	// pseudo-version an application depends on) and falls back to the embedded
	// VERSION file, which only matches when built from a release tag.
	Version string
)

func init() {
	fallback := "development"
	if version, err := data.ReadFile("data/VERSION"); err == nil {
		fallback = strings.TrimSpace(string(version))
	}
	info, ok := debug.ReadBuildInfo()
	Version = resolveVersion(info, ok, fallback)
}

// resolveVersion returns this module's version from build info, without the
// leading "v", or fallback when the build carries no usable version (no build
// info, a "(devel)" checkout, or a local replace directive).
func resolveVersion(info *debug.BuildInfo, ok bool, fallback string) string {
	if !ok || info == nil {
		return fallback
	}
	module := &info.Main
	if module.Path != ModulePath {
		module = nil
		for _, dep := range info.Deps {
			if dep.Path == ModulePath {
				module = dep
				break
			}
		}
	}
	if module != nil && module.Replace != nil {
		// The compiled code is the replacement: a versioned replacement reports its
		// own version, a local (path) replacement has none and uses the fallback.
		module = module.Replace
	}
	if module == nil || module.Version == "" || module.Version == "(devel)" {
		return fallback
	}
	return strings.TrimPrefix(module.Version, "v")
}
