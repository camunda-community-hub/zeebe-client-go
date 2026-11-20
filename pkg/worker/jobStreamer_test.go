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
//

package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/commands"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/stretchr/testify/suite"
)

type JobStreamerSuite struct {
	suite.Suite
}

func TestJobStreamerSuite(t *testing.T) {
	suite.Run(t, new(JobStreamerSuite))
}

func (s *JobStreamerSuite) TestShouldNotOpenStreamIfClosed() {
	// given
	// since we aren't going to stream, the wait group has to be decremented
	// to allow the AfterTest to run properly
	state := newTestState()
	defer state.close(s)
	finished := make(chan bool)
	state.streamer.close()

	// when - should not hang because the stream is closed
	go func() {
		state.streamer.stream(state.waitGroup)
		finished <- true
		close(finished)
	}()

	// then
	select {
	case <-finished:
		s.EqualValues(0, state.command.sendCount)
	case <-time.After(10 * time.Second):
		s.FailNow("Timed out waiting for stream call to finish")
	}
}

func (s *JobStreamerSuite) TestShouldStopStreamOnCloseSignal() {
	// given
	var call sendCall
	state := newTestState()
	defer state.close(s)
	state.command.setSendChanBuffer(0)

	// when - should not hang because the stream is closed
	go state.streamer.stream(state.waitGroup)
	select {
	case call = <-state.command.sendChan:
		state.streamer.closeSignal <- struct{}{}
	case <-time.After(10 * time.Second):
		s.FailNow("Timed out waiting for stream call to finish")
	}

	// then
	select {
	case <-call.ctx.Done():
		s.EqualValues(1, call.n)
	case <-time.After(10 * time.Second):
		s.FailNow("Context was not canceled even though the streamer was closed via a signal")
	}
}

func (s *JobStreamerSuite) TestShouldRecreateStreamOnCompleted() {
	// given
	var call sendCall
	state := newTestState()
	defer state.close(s)
	// will block on the first send call
	state.command.setSendChanBuffer(0)

	// when - should not hang because the stream is closed
	go state.streamer.stream(state.waitGroup)
	select {
	// capture second call context
	case <-state.command.sendChan:
		select {
		case call = <-state.command.sendChan:
			state.streamer.closeSignal <- struct{}{}
		case <-time.After(10 * time.Second):
			s.FailNow("Timed out waiting for stream call to finish")
		}
	case <-time.After(10 * time.Second):
		s.FailNow("Timed out waiting for stream call to finish")
	}

	// then - the second stream was opened by the completion of the first. The
	// streamer reopens again at once, so a third Send may already have started:
	// assert on the captured call, not on the running total.
	select {
	case <-call.ctx.Done():
		s.EqualValues(2, call.n)
		calls, _ := state.backoff.calls()
		s.EqualValues(0, calls)
	case <-time.After(10 * time.Second):
		s.FailNow("Context was not canceled even though the streamer was closed via a signal")
	}
}

func (s *JobStreamerSuite) TestShouldRecreateStreamOnErrorWithBackoff() {
	// given
	var call sendCall
	state := newTestState()
	defer state.close(s)
	state.command.err = errors.New("Foo")
	state.backoff.delaySequence = []time.Duration{1, 2 * time.Hour}
	// will block on the second third call
	state.command.setSendChanBuffer(2)

	// when - should not hang because the stream is closed
	go state.streamer.stream(state.waitGroup)
	select {
	// capture second call context
	case <-state.command.sendChan:
		select {
		case call = <-state.command.sendChan:
			// the second failure backs off for 2h; wait until it has asked for that
			// delay, or the close signal can win the race with the stream result
			s.Eventually(func() bool {
				calls, _ := state.backoff.calls()
				return calls == 2
			}, 10*time.Second, time.Millisecond)
			state.streamer.closeSignal <- struct{}{}
		case <-time.After(10 * time.Second):
			s.FailNow("Timed out waiting for stream call to finish")
		}
	case <-time.After(10 * time.Second):
		s.FailNow("Timed out waiting for stream call to finish")
	}

	// then

	select {
	case <-call.ctx.Done():
		s.EqualValues(2, call.n)
		calls, delays := state.backoff.calls()
		s.EqualValues(2, calls)
		s.Require().Len(delays, 2)
		s.EqualValues(0, delays[0])
		s.EqualValues(1, delays[1])
	case <-time.After(10 * time.Second):
		s.FailNow("Context was not canceled even though the streamer was closed via a signal")
	}
}

type testState struct {
	command   *mockStreamJobsCommand
	backoff   *mockBackoffSupplier
	streamer  *jobStreamer
	jobQueue  chan entities.Job
	waitGroup *sync.WaitGroup
}

func newTestState() *testState {
	command := mockStreamJobsCommand{
		sendChan: make(chan sendCall, 10),
	}
	backoff := mockBackoffSupplier{}
	state := testState{
		command: &command,
		backoff: &backoff,
		streamer: &jobStreamer{
			request:         &command,
			workerFinished:  make(chan bool),
			closeSignal:     make(chan struct{}),
			backoffSupplier: &backoff,
		},
		jobQueue:  make(chan entities.Job),
		waitGroup: &sync.WaitGroup{},
	}

	state.waitGroup.Add(1)

	return &state
}

func (state *testState) close(suite *JobStreamerSuite) {
	close(state.streamer.closeSignal)

	c := make(chan struct{})
	go func() {
		state.waitGroup.Wait()
		close(c)
	}()

	for {
		select {
		case <-c:
			return
		case <-time.After(utils.DefaultTestTimeout):
			suite.FailNow("Failed to wait for job streamer to close")
		}
	}
}

// sendCall is one Send on the mock: its context, and its 1-based call number.
type sendCall struct {
	ctx context.Context
	n   int
}

// The mocks are called from the streamer's goroutines, so every field the test
// reads afterwards is guarded by mu.
type mockStreamJobsCommand struct {
	mu        sync.Mutex
	sendCount int
	err       error
	sendChan  chan sendCall
}

type mockBackoffSupplier struct {
	mu                 sync.Mutex
	calledCount        int
	delaySequence      []time.Duration
	currentRetryDelays []time.Duration
}

func (m *mockStreamJobsCommand) RequestTimeout(_ time.Duration) commands.DispatchStreamJobsCommand {
	panic(errors.New("Not implemented yet as not expected to be invoked"))
}

func (m *mockStreamJobsCommand) Send(ctx context.Context) error {
	m.mu.Lock()
	m.sendCount++
	call := sendCall{ctx: ctx, n: m.sendCount}
	sendChan := m.sendChan
	m.mu.Unlock()

	select {
	case sendChan <- call:
	case <-ctx.Done():
		// like a real stream, a cancelled context ends the call
		return ctx.Err()
	}
	return m.err
}

func (m *mockStreamJobsCommand) setSendChanBuffer(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sendChan := m.sendChan
	m.sendChan = make(chan sendCall, count)
	close(sendChan)
}

func (m *mockBackoffSupplier) SupplyRetryDelay(currentRetryDelay time.Duration) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentRetryDelays = append(m.currentRetryDelays, currentRetryDelay)

	index := m.calledCount
	m.calledCount++
	if index < len(m.delaySequence) {
		return m.delaySequence[index]
	}

	return 0
}

// calls returns how often SupplyRetryDelay was called, and with which delays.
func (m *mockBackoffSupplier) calls() (int, []time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calledCount, append([]time.Duration(nil), m.currentRetryDelays...)
}
