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

package commands

import (
	"os"
	"testing"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/zbc"
)

// isolateAddressEnv unsets the gateway address env vars and flags for the duration of the test,
// restoring them afterwards, so results do not depend on the caller's shell environment.
func isolateAddressEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{zbc.GatewayHostEnvVar, zbc.GatewayPortEnvVar, zbc.GatewayAddressEnvVar} {
		if old, ok := os.LookupEnv(name); ok {
			name, old := name, old
			t.Cleanup(func() { _ = os.Setenv(name, old) })
		} else {
			name := name
			t.Cleanup(func() { _ = os.Unsetenv(name) })
		}
		_ = os.Unsetenv(name)
	}
	oldAddr, oldHost, oldPort := addressFlag, hostFlag, portFlag
	addressFlag, hostFlag, portFlag = "", "", ""
	t.Cleanup(func() { addressFlag, hostFlag, portFlag = oldAddr, oldHost, oldPort })
}

func TestGatewayAddressKeepsIPv6Brackets(t *testing.T) {
	for _, tc := range []struct{ address, want string }{
		{"[::1]:26599", "[::1]:26599"},
		{"[fe80::a00:27ff:fe9b:c00a]:26500", "[fe80::a00:27ff:fe9b:c00a]:26500"},
		{"127.0.0.1:26599", "127.0.0.1:26599"},
		{"[::1]", "[::1]:" + zbc.DefaultAddressPort},
	} {
		t.Run(tc.address, func(t *testing.T) {
			isolateAddressEnv(t)
			addressFlag = tc.address

			got, _, err := gatewayAddress()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("gatewayAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGatewayAddressHostFlagIPv6(t *testing.T) {
	for _, tc := range []struct{ host, port, want string }{
		{"[::1]", "", "[::1]:" + zbc.DefaultAddressPort},
		{"::1", "", "[::1]:" + zbc.DefaultAddressPort},
		{"[::1]", "9090", "[::1]:9090"},
	} {
		t.Run(tc.host+"/"+tc.port, func(t *testing.T) {
			isolateAddressEnv(t)
			hostFlag, portFlag = tc.host, tc.port

			got, host, err := gatewayAddress()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("gatewayAddress() = %q, want %q", got, tc.want)
			}
			if host != "::1" {
				t.Errorf("host = %q, want %q", host, "::1")
			}
		})
	}
}
