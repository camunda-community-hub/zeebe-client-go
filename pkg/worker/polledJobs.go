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

import "sync"

// polledJobs records the keys of jobs the poller activated, so that only those jobs report back
// to the poller when finished. The poller's remaining count covers polled jobs only; finish
// signals for streamed jobs must not reach it (and must not be lost to another reader).
type polledJobs struct {
	mu   sync.Mutex
	keys map[int64]int
}

func newPolledJobs() *polledJobs {
	return &polledJobs{keys: make(map[int64]int)}
}

func (p *polledJobs) add(key int64) {
	p.mu.Lock()
	p.keys[key]++
	p.mu.Unlock()
}

// remove reports whether key was activated by the poller, and forgets one activation of it.
func (p *polledJobs) remove(key int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.keys[key]
	if !ok {
		return false
	}
	if n <= 1 {
		delete(p.keys, key)
	} else {
		p.keys[key] = n - 1
	}
	return true
}
