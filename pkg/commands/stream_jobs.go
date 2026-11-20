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
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

type StreamJobsConsumer chan<- entities.Job

type DispatchStreamJobsCommand interface {
	RequestTimeout(time.Duration) DispatchStreamJobsCommand
	Send(ctx context.Context) error
}

type StreamJobsCommandStep1 interface {
	JobType(string) StreamJobsCommandStep2
}

type StreamJobsCommandStep2 interface {
	Consumer(StreamJobsConsumer) StreamJobsCommandStep3
}

type StreamJobsCommandStep3 interface {
	DispatchStreamJobsCommand

	TenantIds(...string) StreamJobsCommandStep3
	Timeout(time.Duration) StreamJobsCommandStep3
	WorkerName(string) StreamJobsCommandStep3
	FetchVariables(...string) StreamJobsCommandStep3
	// InactivityTimeout cancels and recycles the stream when no job was received on it for the given
	// duration. The timer resets on every received job. Zero or negative disables the watchdog.
	InactivityTimeout(time.Duration) StreamJobsCommandStep3
	// TimeoutJitter randomises the request (max lifetime) and inactivity timeouts of each stream
	// uniformly over [(1-jitter)*timeout, timeout], so a fleet started together does not recycle all of
	// its streams at the same moment. The value is clamped to [0, 1]; zero disables jitter.
	TimeoutJitter(float64) StreamJobsCommandStep3
	// OnRecycle registers a callback invoked whenever the stream is closed because it reached its
	// max lifetime (reason StreamRecycleReasonLifetime) or the inactivity watchdog fired
	// (reason StreamRecycleReasonInactivity).
	OnRecycle(func(reason string)) StreamJobsCommandStep3
	// DebugLogger sets the sink for routine (debug-level) stream lifecycle messages, such as stream
	// recycles. Passing nil disables them. When unset, they go to the standard logger.
	DebugLogger(func(msg string)) StreamJobsCommandStep3
}

const (
	// StreamRecycleReasonLifetime the stream reached its (jittered) max lifetime
	StreamRecycleReasonLifetime = "lifetime"
	// StreamRecycleReasonInactivity no job was received for the (jittered) inactivity timeout
	StreamRecycleReasonInactivity = "inactivity"
)

type StreamJobsCommand struct {
	Command
	request        pb.StreamActivatedJobsRequest
	consumer       StreamJobsConsumer
	requestTimeout time.Duration

	inactivityTimeout time.Duration
	jitter            float64
	onRecycle         func(reason string)
	debugLog          func(msg string)
	debugLogSet       bool
}

func (cmd *StreamJobsCommand) JobType(jobType string) StreamJobsCommandStep2 {
	cmd.request.Type = jobType
	return cmd
}

func (cmd *StreamJobsCommand) Consumer(consumer StreamJobsConsumer) StreamJobsCommandStep3 {
	cmd.consumer = consumer
	return cmd
}

func (cmd *StreamJobsCommand) TenantIds(tenantIds ...string) StreamJobsCommandStep3 {
	cmd.request.TenantIds = tenantIds
	return cmd
}

func (cmd *StreamJobsCommand) Timeout(timeout time.Duration) StreamJobsCommandStep3 {
	cmd.request.Timeout = int64(timeout / time.Millisecond)
	return cmd
}

func (cmd *StreamJobsCommand) WorkerName(workerName string) StreamJobsCommandStep3 {
	cmd.request.Worker = workerName
	return cmd
}

func (cmd *StreamJobsCommand) FetchVariables(fetchVariables ...string) StreamJobsCommandStep3 {
	cmd.request.FetchVariable = fetchVariables
	return cmd
}

func (cmd *StreamJobsCommand) RequestTimeout(requestTimeout time.Duration) DispatchStreamJobsCommand {
	cmd.requestTimeout = requestTimeout
	return cmd
}

func (cmd *StreamJobsCommand) InactivityTimeout(timeout time.Duration) StreamJobsCommandStep3 {
	cmd.inactivityTimeout = timeout
	return cmd
}

func (cmd *StreamJobsCommand) TimeoutJitter(jitter float64) StreamJobsCommandStep3 {
	if jitter < 0 {
		jitter = 0
	} else if jitter > 1 {
		jitter = 1
	}
	cmd.jitter = jitter
	return cmd
}

func (cmd *StreamJobsCommand) OnRecycle(onRecycle func(reason string)) StreamJobsCommandStep3 {
	cmd.onRecycle = onRecycle
	return cmd
}

func (cmd *StreamJobsCommand) DebugLogger(logger func(msg string)) StreamJobsCommandStep3 {
	cmd.debugLog = logger
	cmd.debugLogSet = true
	return cmd
}

// debugf sends a routine (debug-level) stream lifecycle message to the configured DebugLogger sink.
// When DebugLogger was never called, messages go to the standard logger; when it was called with nil,
// they are disabled entirely.
func (cmd *StreamJobsCommand) debugf(format string, args ...interface{}) {
	if cmd.debugLogSet {
		if cmd.debugLog != nil {
			cmd.debugLog(fmt.Sprintf(format, args...))
		}
		return
	}
	log.Printf(format, args...)
}

// JitterDuration returns a duration chosen uniformly from [(1-jitter)*d, d]. Non-positive durations and
// jitter values are returned unchanged.
func JitterDuration(d time.Duration, jitter float64) time.Duration {
	if d <= 0 || jitter <= 0 {
		return d
	}
	if jitter > 1 {
		jitter = 1
	}
	spread := time.Duration(float64(d) * jitter)
	if spread <= 0 {
		return d
	}
	return d - time.Duration(rand.Int63n(int64(spread)+1)) //nolint:gosec // G404: jitter needs no secure random source
}

// Send opens the stream and pushes received jobs to the consumer until the stream ends. When the stream
// is closed because it reached its max lifetime or its inactivity timeout, Send returns nil so the caller
// can reopen it immediately; jobs already received are always handed to the consumer first.
func (cmd *StreamJobsCommand) Send(ctx context.Context) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var reasonMu sync.Mutex
	recycleReason := ""
	recycle := func(reason string) {
		reasonMu.Lock()
		if recycleReason == "" {
			recycleReason = reason
		}
		reasonMu.Unlock()
		cancel()
	}
	recycled := func() string {
		reasonMu.Lock()
		defer reasonMu.Unlock()
		return recycleReason
	}

	if cmd.requestTimeout > 0 {
		lifetime := JitterDuration(cmd.requestTimeout, cmd.jitter)
		lifetimeTimer := time.AfterFunc(lifetime, func() { recycle(StreamRecycleReasonLifetime) })
		defer lifetimeTimer.Stop()
	}

	var inactivityTimer *time.Timer
	inactivity := time.Duration(0)
	if cmd.inactivityTimeout > 0 {
		inactivity = JitterDuration(cmd.inactivityTimeout, cmd.jitter)
		inactivityTimer = time.AfterFunc(inactivity, func() { recycle(StreamRecycleReasonInactivity) })
		defer inactivityTimer.Stop()
	}

	finish := func(err error) error {
		if reason := recycled(); reason != "" && ctx.Err() == nil {
			cmd.debugf("recycling job stream for job type '%s' (reason: %s)\n", cmd.request.Type, reason)
			if cmd.onRecycle != nil {
				cmd.onRecycle(reason)
			}
			return nil
		}
		return err
	}

	stream, err := cmd.openStream(streamCtx)
	if err != nil {
		return finish(err)
	}

	for {
		job, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}

			if recycled() != "" || !cmd.shouldRetry(streamCtx, err) {
				return finish(err)
			}

			stream, err = cmd.openStream(streamCtx)
			if err != nil {
				if finished := finish(err); finished == nil {
					return nil
				}
				log.Printf("Failed to reopen job stream: %v\n", err)
				return err
			}
			continue
		}

		if inactivityTimer != nil {
			inactivityTimer.Stop()
		}
		cmd.consumer <- entities.Job{ActivatedJob: job}
		if inactivityTimer != nil {
			inactivityTimer.Reset(inactivity)
		}
	}

	return finish(nil)
}

func (cmd *StreamJobsCommand) openStream(ctx context.Context) (pb.Gateway_StreamActivatedJobsClient, error) {
	stream, err := cmd.gateway.StreamActivatedJobs(ctx, &cmd.request)

	if err != nil {
		if cmd.shouldRetry(ctx, err) {
			return cmd.openStream(ctx)
		}

		return nil, err
	}

	return stream, nil
}

func NewStreamJobsCommand(gateway pb.GatewayClient, pred retryPredicate) StreamJobsCommandStep1 {
	return &StreamJobsCommand{
		request: pb.StreamActivatedJobsRequest{
			Timeout: DefaultJobTimeoutInMs,
			Worker:  DefaultJobWorkerName,
		},
		Command: Command{
			gateway:     gateway,
			shouldRetry: pred,
		},
	}
}
