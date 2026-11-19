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
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	const fallback = "8.6.0"
	dependency := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Path: "example.com/app", Version: "(devel)"},
			Deps: []*debug.Module{
				{Path: "google.golang.org/grpc", Version: "v1.85.0"},
				{Path: ModulePath, Version: version},
			},
		}
	}

	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"released dependency", dependency("v8.7.0"), true, "8.7.0"},
		{"pseudo-version dependency", dependency("v8.6.1-0.20261119050036-5b0bf18566c1"), true, "8.6.1-0.20261119050036-5b0bf18566c1"},
		{"replaced dependency without version", dependency(""), true, fallback},
		{"main module built from a tag", &debug.BuildInfo{Main: debug.Module{Path: ModulePath, Version: "v8.7.0"}}, true, "8.7.0"},
		{"main module from a checkout", &debug.BuildInfo{Main: debug.Module{Path: ModulePath, Version: "(devel)"}}, true, fallback},
		{"module not in the build", &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}}, true, fallback},
		{"no build info", nil, false, fallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.info, tt.ok, fallback); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
