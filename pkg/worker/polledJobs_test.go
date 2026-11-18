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
	"testing"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

func TestPolledJobsRemoveOnlyKnownKeys(t *testing.T) {
	p := newPolledJobs()
	p.add(1)
	p.add(1) // reactivated after a timeout: two activations of one key
	for i := 1; i <= 2; i++ {
		if !p.remove(1) {
			t.Fatalf("expected activation %d of key 1 to be known", i)
		}
	}
	if p.remove(1) {
		t.Fatal("key 1 should be forgotten after its activations are removed")
	}
	if p.remove(2) {
		t.Fatal("key 2 was never polled")
	}
}

// With streaming enabled, the dispatcher runs polled and streamed jobs from one queue. Only polled
// jobs may signal workerFinished: the poller's remaining count covers polled jobs only.
func TestDispatcherSignalsOnlyPolledJobs(t *testing.T) {
	polled := newPolledJobs()
	dispatcher := jobDispatcher{
		jobQueue:       make(chan entities.Job),
		workerFinished: make(chan bool, 10),
		closeSignal:    make(chan struct{}),
		polled:         polled,
	}
	var handled sync.WaitGroup
	handler := func(JobClient, entities.Job) { handled.Done() }

	var closeWait sync.WaitGroup
	closeWait.Add(1)
	go dispatcher.run(&jobClientStub{}, handler, 2, &closeWait)

	polled.add(1)
	polled.add(2)
	handled.Add(4)
	for _, key := range []int64{1, 3, 2, 4} { // 3 and 4 arrive from the stream
		dispatcher.jobQueue <- entities.Job{ActivatedJob: &pb.ActivatedJob{Key: key}}
	}
	handled.Wait()
	close(dispatcher.closeSignal)
	closeWait.Wait()

	if got := len(dispatcher.workerFinished); got != 2 {
		t.Fatalf("expected 2 workerFinished signals (polled jobs only), got %d", got)
	}
}

// Without streaming (polled == nil), every finished job signals, as before.
func TestDispatcherSignalsEveryJobWithoutTracking(t *testing.T) {
	dispatcher := jobDispatcher{
		jobQueue:       make(chan entities.Job),
		workerFinished: make(chan bool, 10),
		closeSignal:    make(chan struct{}),
	}
	var handled sync.WaitGroup
	handler := func(JobClient, entities.Job) { handled.Done() }

	var closeWait sync.WaitGroup
	closeWait.Add(1)
	go dispatcher.run(&jobClientStub{}, handler, 2, &closeWait)

	handled.Add(3)
	for _, key := range []int64{1, 2, 3} {
		dispatcher.jobQueue <- entities.Job{ActivatedJob: &pb.ActivatedJob{Key: key}}
	}
	handled.Wait()
	// handled.Done runs inside the handler, before the dispatcher sends its signal
	deadline := time.Now().Add(time.Second)
	for len(dispatcher.workerFinished) < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(dispatcher.closeSignal)
	closeWait.Wait()

	if got := len(dispatcher.workerFinished); got != 3 {
		t.Fatalf("expected 3 workerFinished signals, got %d", got)
	}
}
