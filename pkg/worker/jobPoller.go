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
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type jobPoller struct {
	client              pb.GatewayClient
	request             *pb.ActivateJobsRequest
	requestTimeout      time.Duration
	maxJobsActive       int
	initialPollInterval time.Duration
	pollInterval        time.Duration

	jobQueue       chan entities.Job
	workerFinished chan bool
	closeSignal    chan struct{}
	// polled, if set, records each polled activation so only polled jobs signal workerFinished.
	polled      *polledJobs
	remaining   int
	threshold   int
	metrics     JobWorkerMetrics
	shouldRetry func(context.Context, error) bool

	backoffSupplier BackoffSupplier
	// streamNoJobsBackoffSupplier, if set, backs off the poll interval whenever a poll succeeds but
	// activates no jobs. It is only set when job streaming is enabled, mirroring the Camunda Java
	// client: streamed jobs arrive via the stream, so the backup poll should cost little when idle.
	streamNoJobsBackoffSupplier BackoffSupplier

	// ctx is cancelled when the poller is closed, aborting any in-flight ActivateJobs call.
	ctx context.Context
}

func (poller *jobPoller) poll(closeWait *sync.WaitGroup) {
	defer closeWait.Done()

	// derive the activation context from the close signal, so that closing the worker cancels an
	// in-flight long poll instead of letting it activate jobs nobody will handle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closeSignal := poller.closeSignal
	go func() {
		select {
		case <-closeSignal:
			cancel()
		case <-ctx.Done():
		}
	}()
	poller.ctx = ctx

	// initial poll
	poller.activateJobs()

	for {
		select {
		// either a job was finished
		case <-poller.workerFinished:
			poller.remaining--
			poller.setJobsRemainingCountMetric(poller.remaining)
			poller.pollInterval = poller.initialPollInterval
		// or the poll interval exceeded
		case <-time.After(poller.pollInterval):
		// or poller should stop
		case <-poller.closeSignal:
			poller.setJobsRemainingCountMetric(0)
			return
		}

		if poller.isClosed() {
			poller.setJobsRemainingCountMetric(0)
			return
		}

		if poller.shouldActivateJobs() {
			poller.activateJobs()
		}
	}
}

func (poller *jobPoller) shouldActivateJobs() bool {
	return poller.remaining <= poller.threshold
}

func (poller *jobPoller) isClosed() bool {
	select {
	case <-poller.closeSignal:
		return true
	default:
		return false
	}
}

func (poller *jobPoller) activateJobs() {
	// never start an activation once closing: the cancel goroutine may not have
	// run yet, so the context alone can still be live (e.g. Close before the
	// initial poll)
	if poller.isClosed() {
		return
	}
	parent := poller.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, poller.requestTimeout)
	defer cancel()

	poller.request.MaxJobsToActivate = int32(poller.maxJobsActive - poller.remaining)
	stream, err := poller.openStream(ctx)
	if err != nil {
		if !poller.isClosed() {
			log.Println(err.Error())
		}
		return
	}

	activated := 0
	failed := false
	defer func() {
		if !failed {
			poller.onPollSuccess(activated)
		}
	}()

	for {
		response, err := stream.Recv()
		if err != nil {
			// no need to retry if the error was simply that we reached the end of the stream
			if err == io.EOF {
				break
			}

			// the worker is closing and the request was cancelled: stop without retrying
			if poller.isClosed() {
				break
			}

			if poller.shouldRetry(ctx, err) {
				// the headers are outdated and need to be rebuilt
				stream, err = poller.openStream(ctx)
				if err != nil {
					log.Printf("Failed to reopen job polling stream: %v\n", err)
					// the poll did not complete successfully, so it must not adjust the
					// poll interval as a success (empty-poll backoff or reset) would.
					failed = true
					break
				}
				continue
			}

			if err != io.EOF && status.Code(err) != codes.ResourceExhausted {
				log.Printf("Failed to activate jobs for worker '%s': %v\n", poller.request.Worker, err)
			}

			failed = true
			switch status.Code(err) {
			case codes.ResourceExhausted, codes.Unavailable, codes.Internal:
				poller.backoff()
			}

			break
		}

		activated += len(response.Jobs)
		poller.remaining += len(response.Jobs)
		poller.setJobsRemainingCountMetric(poller.remaining)
		for _, job := range response.Jobs {
			if poller.polled != nil {
				poller.polled.add(job)
			}
			poller.jobQueue <- entities.Job{ActivatedJob: job}
		}
	}
}

func (poller *jobPoller) openStream(ctx context.Context) (pb.Gateway_ActivateJobsClient, error) {
	stream, err := poller.client.ActivateJobs(ctx, poller.request)
	if err != nil {
		if ctx.Err() == nil && !poller.isClosed() && poller.shouldRetry(ctx, err) {
			return poller.openStream(ctx)
		}
		return nil, fmt.Errorf("worker '%s' failed to open job stream: %w", poller.request.Worker, err)
	}

	return stream, nil
}

func (poller *jobPoller) setJobsRemainingCountMetric(count int) {
	if poller.metrics != nil {
		poller.metrics.SetJobsRemainingCount(poller.request.GetType(), count)
	}
}

func (poller *jobPoller) backoff() {
	prevInterval := poller.pollInterval
	poller.pollInterval = poller.backoffSupplier.SupplyRetryDelay(prevInterval)
}

// onPollSuccess adjusts the poll interval after a poll completed without error. With streaming
// enabled, an empty poll backs off the interval (exponentially, from the current interval), while a
// poll that activated jobs resets it to the configured interval so backlogs drain at full speed.
func (poller *jobPoller) onPollSuccess(activated int) {
	if poller.streamNoJobsBackoffSupplier == nil {
		return
	}
	if activated == 0 {
		poller.pollInterval = poller.streamNoJobsBackoffSupplier.SupplyRetryDelay(poller.pollInterval)
	} else {
		poller.pollInterval = poller.initialPollInterval
	}
}
