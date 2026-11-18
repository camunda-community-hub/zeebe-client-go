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

func TestPolledJobsRemoveOnlyKnownActivations(t *testing.T) {
	p := newPolledJobs()
	a1, a2, other := &pb.ActivatedJob{Key: 1}, &pb.ActivatedJob{Key: 1}, &pb.ActivatedJob{Key: 2}
	p.add(a1)
	p.add(a2) // same key reactivated after a timeout: two distinct activations
	for i, activation := range []*pb.ActivatedJob{a1, a2} {
		if !p.remove(activation) {
			t.Fatalf("expected activation %d of key 1 to be known", i+1)
		}
	}
	if p.remove(a1) {
		t.Fatal("activation a1 should be forgotten after it was removed")
	}
	if p.remove(other) {
		t.Fatal("key 2 was never polled")
	}
}

// A streamed job and a polled job may share a key when the streamed activation times out and the
// poller reactivates the job while the streamed copy is still running. The streamed completion
// must not consume the polled activation's entry: the poller's remaining count covers the polled
// activation until the polled work itself finishes.
func TestPolledJobsSameKeyStreamedActivationDoesNotConsumePolledEntry(t *testing.T) {
	p := newPolledJobs()
	polledActivation := &pb.ActivatedJob{Key: 1}
	streamedActivation := &pb.ActivatedJob{Key: 1}
	p.add(polledActivation)

	if p.remove(streamedActivation) {
		t.Fatal("streamed activation of the same key must not consume the polled entry")
	}
	if !p.remove(polledActivation) {
		t.Fatal("polled activation should still be tracked after the streamed completion")
	}
	if p.remove(polledActivation) {
		t.Fatal("polled activation should be forgotten after it was removed")
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

	job1 := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 1}}
	job2 := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 2}}
	job3 := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 3}}
	job4 := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 4}}
	polled.add(job1.ActivatedJob)
	polled.add(job2.ActivatedJob)
	handled.Add(4)
	for _, job := range []entities.Job{job1, job3, job2, job4} { // 3 and 4 arrive from the stream
		dispatcher.jobQueue <- job
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

// The streamed activation of a job times out, the poller reactivates the same key, and the
// streamed handler returns first. Its completion must not signal the poller: only the polled
// activation's completion may decrement the poller's remaining count.
func TestDispatcherSameKeyStreamedCompletionDoesNotSignalPoller(t *testing.T) {
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

	streamed := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 1}}
	polledCopy := entities.Job{ActivatedJob: &pb.ActivatedJob{Key: 1}} // reactivated after timeout
	polled.add(polledCopy.ActivatedJob)

	handled.Add(2)
	dispatcher.jobQueue <- streamed
	dispatcher.jobQueue <- polledCopy
	handled.Wait()
	// handled.Done runs inside the handler, before the dispatcher sends its signal
	deadline := time.Now().Add(time.Second)
	for len(dispatcher.workerFinished) < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(dispatcher.closeSignal)
	closeWait.Wait()

	if got := len(dispatcher.workerFinished); got != 1 {
		t.Fatalf("expected exactly 1 workerFinished signal (the polled activation only), got %d", got)
	}
}
