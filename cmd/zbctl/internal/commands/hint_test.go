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
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConnectionHint(t *testing.T) {
	// The error messages are the ones zbctl printed against a plaintext Zeebe broker and
	// against a TLS-only endpoint.
	tlsToPlaintext := status.Error(codes.Unavailable, `connection error: desc = "transport: authentication handshake failed: tls: first record does not look like a TLS handshake"`)
	plaintextToTLS := status.Error(codes.DeadlineExceeded, "context deadline exceeded while waiting for connections to become ready")
	refused := status.Error(codes.Unavailable, `connection error: desc = "transport: Error while dialing: dial tcp 127.0.0.1:26500: connect: connection refused"`)

	for _, tc := range []struct {
		name     string
		err      error
		insecure bool
		want     string
	}{
		{name: "TLS client, plaintext gateway", err: tlsToPlaintext, want: "--insecure"},
		{name: "plaintext client, TLS gateway", err: plaintextToTLS, insecure: true, want: "remove --insecure"},
		{name: "TLS mismatch error but already insecure", err: tlsToPlaintext, insecure: true},
		{name: "timeout without --insecure", err: plaintextToTLS},
		{name: "nothing listening", err: refused},
		{name: "unrelated error", err: errors.New("boom")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := connectionHint(tc.err, tc.insecure)
			if tc.want == "" && got != "" {
				t.Errorf("connectionHint() = %q, want no hint", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("connectionHint() = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}
