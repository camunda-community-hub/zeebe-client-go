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
	"strings"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/zbc"
)

// connectionHint suggests the --insecure flag when an error looks like a TLS mismatch
// between zbctl and the gateway. Zeebe gateways run without TLS by default, while zbctl
// uses TLS by default, so this is a common first-run problem. It is only a hint: the
// errors are not specific enough to change the connection automatically.
func connectionHint(err error, insecure bool) string {
	msg := err.Error()
	switch {
	case !insecure && strings.Contains(msg, "tls: first record does not look like a TLS handshake"):
		return "Hint: the gateway does not seem to use TLS. If it is a local or development gateway, " +
			"add the --insecure flag (or set " + zbc.InsecureEnvVar + "=true)."
	case insecure && strings.Contains(msg, "while waiting for connections to become ready"):
		return "Hint: zbctl could not connect without TLS. If the gateway uses TLS (for example Camunda SaaS), " +
			"remove --insecure (and unset " + zbc.InsecureEnvVar + ")."
	}
	return ""
}

// usesInsecureConnection reports whether zbctl connects without TLS, from the flag or the
// environment.
func usesInsecureConnection() bool {
	return insecureFlag || os.Getenv(zbc.InsecureEnvVar) == "true"
}
