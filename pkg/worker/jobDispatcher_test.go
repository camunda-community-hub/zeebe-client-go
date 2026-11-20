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

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/commands"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/stretchr/testify/suite"
)

type JobDispatcherSuite struct {
	suite.Suite
	client          jobClientStub
	dispatcher      jobDispatcher
	awaitHandler    chan bool
	continueHandler chan bool
	waitGroup       sync.WaitGroup
}

func (suite *JobDispatcherSuite) BeforeTest(string, string) {
	suite.client = jobClientStub{}
	suite.dispatcher = jobDispatcher{
		jobQueue:       make(chan entities.Job),
		workerFinished: make(chan bool),
		closeSignal:    make(chan struct{}),
	}
	suite.awaitHandler = make(chan bool)
	suite.continueHandler = make(chan bool)
	suite.waitGroup.Add(1)
}

func (suite *JobDispatcherSuite) AfterTest(string, string) {
	c := make(chan struct{})
	go func() {
		suite.waitGroup.Wait()
		close(c)
	}()

	for {
		select {
		case <-c:
			return
		case <-suite.awaitHandler:
			// complete jobs
			suite.completeJob(false)
		case <-time.After(utils.DefaultTestTimeout):
			suite.FailNow("Failed to wait for job dispatcher to close")
		}
	}
}

func TestJobDispatcherSuite(t *testing.T) {
	suite.Run(t, new(JobDispatcherSuite))
}

func (suite *JobDispatcherSuite) TestShouldCloseDispatcherWhenWaitingForJob() {
	// given
	handler := suite.newSyncedJobHandler()

	go suite.dispatcher.run(&suite.client, handler, 1, &suite.waitGroup)

	// when
	suite.dispatcher.jobQueue <- entities.Job{}
	// await handler received job so the dispatcher will wait for next job
	<-suite.awaitHandler

	// then
	close(suite.dispatcher.closeSignal)
	suite.completeJob(false)
}

func (suite *JobDispatcherSuite) TestShouldCloseDispatcherWhenWaitingForWorker() {
	// given
	handler := suite.newSyncedJobHandler()

	go suite.dispatcher.run(&suite.client, handler, 1, &suite.waitGroup)

	// when
	suite.dispatcher.jobQueue <- entities.Job{}
	// await the handler ist blocked
	<-suite.awaitHandler
	// await the dispatcher picked up the next job and waits for worker
	suite.dispatcher.jobQueue <- entities.Job{}

	// then
	close(suite.dispatcher.closeSignal)
	suite.completeJob(false)
}

func (suite *JobDispatcherSuite) TestShouldReuseHandler() {
	// given
	handler := suite.newSyncedJobHandler()

	go suite.dispatcher.run(&suite.client, handler, 1, &suite.waitGroup)

	// when
	suite.dispatcher.jobQueue <- entities.Job{}
	suite.dispatcher.jobQueue <- entities.Job{}

	// then
	suite.completeJob(true)
	suite.completeJob(true)

	close(suite.dispatcher.closeSignal)
}

func (suite *JobDispatcherSuite) TestShouldPassClientAndJobToHandler() {
	// given
	var jobKey int64 = 123

	handler := func(client JobClient, job entities.Job) {
		suite.Assert().Equal(jobKey, job.Key)
		client.NewCompleteJobCommand()
	}

	go suite.dispatcher.run(&suite.client, handler, 1, &suite.waitGroup)

	// when
	suite.dispatcher.jobQueue <- entities.Job{ActivatedJob: &pb.ActivatedJob{Key: jobKey}}

	// then
	select {
	case <-suite.dispatcher.workerFinished:
		suite.Assert().True(suite.client.invoked)
	case <-time.After(utils.DefaultTestTimeout):
		suite.FailNow("Failed to wait for job handler invocation")
	}

	close(suite.dispatcher.closeSignal)
}

func (suite *JobDispatcherSuite) TestShouldHandleMultipleJobsInParallel() {
	// given
	concurrency := 4
	handler := suite.newSyncedJobHandler()

	go suite.dispatcher.run(&suite.client, handler, concurrency, &suite.waitGroup)

	// when
	for i := 0; i < concurrency; i++ {
		suite.dispatcher.jobQueue <- entities.Job{}
	}

	// then
	for i := 0; i < concurrency; i++ {
		suite.completeJob(true)
	}

	close(suite.dispatcher.closeSignal)
}

func (suite *JobDispatcherSuite) newSyncedJobHandler() func(JobClient, entities.Job) {
	return func(JobClient, entities.Job) {
		suite.awaitHandler <- true
		select {
		case <-suite.continueHandler:
			break
		case <-time.After(utils.DefaultTestTimeout):
			suite.FailNow("Failed to wait for job handler sync")
		}
	}
}

func (suite *JobDispatcherSuite) completeJob(syncHandler bool) {
	if syncHandler {
		<-suite.awaitHandler
	}
	suite.continueHandler <- true
	<-suite.dispatcher.workerFinished
}

// A worker that advertised itself in workerQueue but has not yet received its job must still run
// the handler when the dispatcher drains and closes. With a buffered work channel the dispatcher's
// send completes before the worker receives, the deferred close(closeWorkers) can then win the
// worker's next select, and the job is stranded unhandled. The unbuffered work channel makes the
// handoff synchronous, so the drain cannot return before the worker has received the job.
func TestJobDispatcherDrainCannotOvertakeWorkerReceipt(t *testing.T) {
	for i := 0; i < 100; i++ {
		queue := make(chan entities.Job, 1)
		queue <- entities.Job{ActivatedJob: &pb.ActivatedJob{Key: int64(i)}}
		dispatcher := jobDispatcher{
			jobQueue:       queue,
			workerFinished: make(chan bool, 1),
			closeSignal:    make(chan struct{}),
		}
		// already closed: the dispatcher only runs its drain path, so the handoff races the
		// deferred close(closeWorkers) as tightly as possible
		close(dispatcher.closeSignal)

		handled := make(chan int64, 1)
		var wg sync.WaitGroup
		wg.Add(1)
		go dispatcher.run(&jobClientStub{}, func(_ JobClient, job entities.Job) {
			handled <- job.Key
		}, 1, &wg)

		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(utils.DefaultTestTimeout):
			t.Fatal("dispatcher did not close")
		}

		select {
		case key := <-handled:
			if key != int64(i) {
				t.Fatalf("handled wrong job: got %d, want %d", key, i)
			}
		default:
			t.Fatalf("iteration %d: shutdown overtook the handoff; job was stranded", i)
		}
	}
}

type jobClientStub struct {
	invoked bool
}

func (stub *jobClientStub) NewCompleteJobCommand() commands.CompleteJobCommandStep1 {
	stub.invoked = true
	return nil
}

func (stub *jobClientStub) NewThrowErrorCommand() commands.ThrowErrorCommandStep1 {
	panic("implement me")
}

func (jobClientStub) NewFailJobCommand() commands.FailJobCommandStep1 {
	panic("implement me")
}

func TestJobDispatcherDrainsQueueOnClose(t *testing.T) {
	queue := make(chan entities.Job, 3)
	for i := int64(1); i <= 3; i++ {
		queue <- entities.Job{ActivatedJob: &pb.ActivatedJob{Key: i}}
	}
	dispatcher := jobDispatcher{
		jobQueue:       queue,
		workerFinished: make(chan bool, 3),
		closeSignal:    make(chan struct{}),
	}
	close(dispatcher.closeSignal)

	var mu sync.Mutex
	var handled []int64
	var wg sync.WaitGroup
	wg.Add(1)
	go dispatcher.run(&jobClientStub{}, func(_ JobClient, job entities.Job) {
		mu.Lock()
		handled = append(handled, job.Key)
		mu.Unlock()
	}, 1, &wg)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("dispatcher did not close")
	}
	if len(handled) != 3 {
		t.Fatalf("expected all 3 queued jobs to be handled on close, got %v", handled)
	}
}
