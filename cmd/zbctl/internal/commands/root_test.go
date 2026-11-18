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

import "testing"

func TestGatewayAddressKeepsIPv6Brackets(t *testing.T) {
	for _, tc := range []struct{ address, want string }{
		{"[::1]:26599", "[::1]:26599"},
		{"[fe80::a00:27ff:fe9b:c00a]:26500", "[fe80::a00:27ff:fe9b:c00a]:26500"},
		{"127.0.0.1:26599", "127.0.0.1:26599"},
	} {
		t.Run(tc.address, func(t *testing.T) {
			addressFlag = tc.address
			t.Cleanup(func() { addressFlag = "" })

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
