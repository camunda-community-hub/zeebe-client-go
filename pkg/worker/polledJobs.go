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

package worker

import (
	"sync"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

// polledJobs records the activations the poller received, so that only those jobs report back to
// the poller when finished. The poller's remaining count covers polled jobs only; finish signals
// for streamed jobs must not reach it (and must not be lost to another reader).
//
// Activations are tracked by their *pb.ActivatedJob pointer, not by job key: when a streamed
// activation of a job times out, the poller may reactivate the same key while the streamed copy
// is still being handled, and key-based tracking would let the streamed completion consume the
// polled activation's entry, decrementing the poller's remaining count before the polled work
// actually finished.
type polledJobs struct {
	mu          sync.Mutex
	activations map[*pb.ActivatedJob]struct{}
}

func newPolledJobs() *polledJobs {
	return &polledJobs{activations: make(map[*pb.ActivatedJob]struct{})}
}

func (p *polledJobs) add(job *pb.ActivatedJob) {
	p.mu.Lock()
	p.activations[job] = struct{}{}
	p.mu.Unlock()
}

// remove reports whether job was activated by the poller, and forgets that activation.
func (p *polledJobs) remove(job *pb.ActivatedJob) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.activations[job]; !ok {
		return false
	}
	delete(p.activations, job)
	return true
}
