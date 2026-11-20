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
	"log"
	"math"
	"sync"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/commands"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

const (
	DefaultJobWorkerMaxJobActive  = 32
	DefaultJobWorkerConcurrency   = 4
	DefaultJobWorkerPollInterval  = 100 * time.Millisecond
	DefaultJobWorkerPollThreshold = 0.3
	RequestTimeoutOffset          = 10 * time.Second
	DefaultRequestTimeout         = 10 * time.Second
	DefaultStreamEnabled          = false
	// DefaultStreamRequestTimeout is the default max lifetime of a job stream; when it is reached the
	// stream is closed and immediately reopened, which lets streams rebalance across gateways.
	DefaultStreamRequestTimeout = 8 * time.Hour
	// DefaultStreamInactivityTimeout is the default time without any received job after which a job
	// stream is considered stalled and is recreated.
	DefaultStreamInactivityTimeout = 10 * time.Minute
	// DefaultStreamTimeoutJitter spreads each stream's max lifetime and inactivity timeout uniformly
	// over [0.9x, 1.0x] of the configured value.
	DefaultStreamTimeoutJitter = 0.1
)

var defaultBackoffSupplier = NewExponentialBackoffBuilder().Build()

// maxStreamNoJobsPollDelay caps the backup poll interval of a streaming worker (Camunda Java
// client default).
const maxStreamNoJobsPollDelay = time.Minute

var defaultStreamNoJobsBackoff = NewExponentialBackoffBuilder().MaxDelay(maxStreamNoJobsPollDelay).Build()

// DefaultStreamNoJobsBackoffSupplier backs off the backup poll of a streaming worker while polls
// return no jobs: exponential from the current poll interval, factor 1.6, jitter 0.1, capped at
// 1 minute (Camunda Java client defaults). The jitter is applied before the cap, so the returned
// delay never exceeds 1 minute.
var DefaultStreamNoJobsBackoffSupplier = backoffSupplierFunc(func(currentRetryDelay time.Duration) time.Duration {
	delay := defaultStreamNoJobsBackoff.SupplyRetryDelay(currentRetryDelay)
	return min(delay, maxStreamNoJobsPollDelay)
})

// backoffSupplierFunc adapts a function to the BackoffSupplier interface.
type backoffSupplierFunc func(currentRetryDelay time.Duration) time.Duration

func (f backoffSupplierFunc) SupplyRetryDelay(currentRetryDelay time.Duration) time.Duration {
	return f(currentRetryDelay)
}

type JobWorkerBuilder struct {
	gatewayClient  pb.GatewayClient
	jobClient      JobClient
	request        *pb.ActivateJobsRequest
	requestTimeout time.Duration

	handler              JobHandler
	maxJobsActive        int
	concurrency          int
	pollInterval         time.Duration
	pollThreshold        float64
	metrics              JobWorkerMetrics
	shouldRetry          func(context.Context, error) bool
	backoffSupplier      BackoffSupplier
	streamEnabled        bool
	streamNoJobsBackoff  BackoffSupplier
	streamRequestTimeout time.Duration

	streamInactivityTimeout time.Duration
	streamTimeoutJitter     float64
	streamDebugLogger       func(msg string)
}

// JobWorkerStreamMetrics can optionally be implemented by a JobWorkerMetrics to count job stream recycles.
// reason is commands.StreamRecycleReasonLifetime or commands.StreamRecycleReasonInactivity.
type JobWorkerStreamMetrics interface {
	IncrementStreamRecycledCount(jobType string, reason string)
}

type JobWorkerBuilderStep1 interface {
	// JobType Set the type of jobs to work on
	JobType(string) JobWorkerBuilderStep2
}

type JobWorkerBuilderStep2 interface {
	// Handler Set the handler to process jobs. The worker should complete or fail the job. The handler implementation
	// must be thread-safe.
	Handler(JobHandler) JobWorkerBuilderStep3
}

type JobWorkerBuilderStep3 interface {
	// Name Set the name of the worker owner
	Name(string) JobWorkerBuilderStep3
	// Timeout Set the duration no other worker should work on job activated by this worker
	Timeout(time.Duration) JobWorkerBuilderStep3
	// RequestTimeout Set the timeout for the request
	RequestTimeout(time.Duration) JobWorkerBuilderStep3
	// MaxJobsActive Set the maximum number of jobs which will be activated for this worker at the
	// same time.
	MaxJobsActive(int) JobWorkerBuilderStep3
	// Concurrency Set the maximum number of concurrent spawned goroutines to complete jobs
	Concurrency(int) JobWorkerBuilderStep3
	// PollInterval Set the maximal interval between polling for new jobs
	PollInterval(time.Duration) JobWorkerBuilderStep3
	// PollThreshold Set the threshold of buffered activated jobs before polling for new jobs, i.e. threshold * MaxJobsActive(int)
	PollThreshold(float64) JobWorkerBuilderStep3
	// FetchVariables Set list of variable names which should be fetched on job activation
	FetchVariables(...string) JobWorkerBuilderStep3
	// a list of IDs of tenants for which to activate jobs
	TenantIds(...string) JobWorkerBuilderStep3
	// Metrics Set implementation for metrics reporting
	Metrics(metrics JobWorkerMetrics) JobWorkerBuilderStep3
	// BackoffSupplier Set the backoffSupplier to back off polling on errors
	BackoffSupplier(supplier BackoffSupplier) JobWorkerBuilderStep3
	// StreamEnabled Enables the job worker to stream jobs. It will still poll for older jobs, but streaming is favored.
	StreamEnabled(bool) JobWorkerBuilderStep3
	// StreamNoJobsBackoffSupplier If streaming is enabled, backs off the backup poll with this supplier
	// while polls return no jobs; a poll that activates jobs resets the interval to PollInterval.
	// Defaults to DefaultStreamNoJobsBackoffSupplier. Pass nil to disable (poll at a fixed interval).
	StreamNoJobsBackoffSupplier(BackoffSupplier) JobWorkerBuilderStep3
	// StreamRequestTimeout If streaming is enabled, this sets the max lifetime of the underlying job stream
	// (default DefaultStreamRequestTimeout, 8h, jittered). When it is reached, the stream is closed and
	// immediately reopened, which lets streams load-balance across gateways over time. Zero or a negative
	// value disables it.
	StreamRequestTimeout(time.Duration) JobWorkerBuilderStep3
	// StreamInactivityTimeout If streaming is enabled, the stream is cancelled and recreated when no job was
	// received on it for this duration (default DefaultStreamInactivityTimeout, 10m, jittered). The timer
	// resets on every received job. It must be less than the stream request timeout, otherwise it is
	// disabled. Zero or a negative value disables the watchdog.
	StreamInactivityTimeout(time.Duration) JobWorkerBuilderStep3
	// StreamTimeoutJitter Sets the fraction (0..1, default DefaultStreamTimeoutJitter) by which each
	// stream's request and inactivity timeouts are randomly shortened, so streams of workers started
	// together are not all recycled at the same moment. Zero disables jitter.
	StreamTimeoutJitter(float64) JobWorkerBuilderStep3
	// StreamDebugLogger sets the sink for routine (debug-level) stream lifecycle messages, such as
	// stream recycles. Passing nil disables them. When unset, they go to the standard logger.
	StreamDebugLogger(func(msg string)) JobWorkerBuilderStep3
	// Open the job worker and start polling and handling jobs
	Open() JobWorker
}

func (builder *JobWorkerBuilder) JobType(jobType string) JobWorkerBuilderStep2 {
	builder.request.Type = jobType
	return builder
}

func (builder *JobWorkerBuilder) Handler(handler JobHandler) JobWorkerBuilderStep3 {
	builder.handler = handler
	return builder
}

func (builder *JobWorkerBuilder) Name(name string) JobWorkerBuilderStep3 {
	builder.request.Worker = name
	return builder
}

func (builder *JobWorkerBuilder) Timeout(timeout time.Duration) JobWorkerBuilderStep3 {
	builder.request.Timeout = int64(timeout / time.Millisecond)
	return builder
}

func (builder *JobWorkerBuilder) RequestTimeout(timeout time.Duration) JobWorkerBuilderStep3 {
	builder.request.RequestTimeout = int64(timeout / time.Millisecond)
	builder.requestTimeout = timeout + RequestTimeoutOffset
	return builder
}

func (builder *JobWorkerBuilder) MaxJobsActive(maxJobsActive int) JobWorkerBuilderStep3 {
	if maxJobsActive > 0 {
		builder.maxJobsActive = maxJobsActive
	} else {
		log.Println("Ignoring invalid maximum", maxJobsActive, "which should be greater then for job worker and using instead", builder.maxJobsActive)
	}
	return builder
}

func (builder *JobWorkerBuilder) Concurrency(concurrency int) JobWorkerBuilderStep3 {
	if concurrency > 0 {
		builder.concurrency = concurrency
	} else {
		log.Println("Ignoring invalid concurrency", concurrency, "which should be greater then zero for job worker and using instead", builder.concurrency)
	}
	return builder
}

func (builder *JobWorkerBuilder) PollInterval(pollInterval time.Duration) JobWorkerBuilderStep3 {
	builder.pollInterval = pollInterval
	return builder
}

func (builder *JobWorkerBuilder) PollThreshold(pollThreshold float64) JobWorkerBuilderStep3 {
	if pollThreshold > 0 {
		builder.pollThreshold = pollThreshold
	} else {
		log.Println("Ignoring invalid poll threshold", pollThreshold, "which should be greater then zero for job worker and using instead", builder.concurrency)
	}
	return builder
}

func (builder *JobWorkerBuilder) FetchVariables(fetchVariables ...string) JobWorkerBuilderStep3 {
	builder.request.FetchVariable = fetchVariables
	return builder
}

func (builder *JobWorkerBuilder) TenantIds(tenantIds ...string) JobWorkerBuilderStep3 {
	builder.request.TenantIds = tenantIds
	return builder
}

func (builder *JobWorkerBuilder) Metrics(metrics JobWorkerMetrics) JobWorkerBuilderStep3 {
	builder.metrics = metrics
	return builder
}

func (builder *JobWorkerBuilder) BackoffSupplier(backoffSupplier BackoffSupplier) JobWorkerBuilderStep3 {
	builder.backoffSupplier = backoffSupplier
	return builder
}

func (builder *JobWorkerBuilder) StreamEnabled(streamEnabled bool) JobWorkerBuilderStep3 {
	builder.streamEnabled = streamEnabled
	return builder
}

func (builder *JobWorkerBuilder) StreamNoJobsBackoffSupplier(supplier BackoffSupplier) JobWorkerBuilderStep3 {
	builder.streamNoJobsBackoff = supplier
	return builder
}

func (builder *JobWorkerBuilder) StreamRequestTimeout(requestTimeout time.Duration) JobWorkerBuilderStep3 {
	builder.streamRequestTimeout = requestTimeout
	return builder
}

func (builder *JobWorkerBuilder) StreamInactivityTimeout(timeout time.Duration) JobWorkerBuilderStep3 {
	builder.streamInactivityTimeout = timeout
	return builder
}

func (builder *JobWorkerBuilder) StreamTimeoutJitter(jitter float64) JobWorkerBuilderStep3 {
	builder.streamTimeoutJitter = jitter
	return builder
}

func (builder *JobWorkerBuilder) StreamDebugLogger(logger func(msg string)) JobWorkerBuilderStep3 {
	builder.streamDebugLogger = logger
	return builder
}

// effectiveStreamInactivityTimeout returns the inactivity timeout to use, disabling it when it is not
// less than the stream request timeout.
func (builder *JobWorkerBuilder) effectiveStreamInactivityTimeout() time.Duration {
	inactivity := builder.streamInactivityTimeout
	if inactivity > 0 && builder.streamRequestTimeout > 0 && inactivity >= builder.streamRequestTimeout {
		log.Printf("Stream inactivity timeout %v must be less than the stream request timeout %v; disabling the inactivity watchdog for job type '%s'\n",
			inactivity, builder.streamRequestTimeout, builder.request.Type)
		return 0
	}
	return inactivity
}

func (builder *JobWorkerBuilder) Open() JobWorker {
	jobQueue := make(chan entities.Job, builder.maxJobsActive)
	workerFinished := make(chan bool, builder.maxJobsActive)
	closePoller := make(chan struct{})
	closeDispatcher := make(chan struct{})
	closeStreamer := make(chan struct{})
	var closeWait, acquireWait sync.WaitGroup
	closeWait.Add(1)
	acquireWait.Add(2)

	// With streaming, the dispatcher also runs streamed jobs; only polled jobs may count down the
	// poller's remaining jobs, so track which jobs the poller activated.
	var polled *polledJobs
	if builder.streamEnabled {
		polled = newPolledJobs()
	}

	poller := jobPoller{
		client:              builder.gatewayClient,
		maxJobsActive:       builder.maxJobsActive,
		pollInterval:        builder.pollInterval,
		initialPollInterval: builder.pollInterval,
		request:             builder.request,
		requestTimeout:      builder.requestTimeout,

		jobQueue:        jobQueue,
		workerFinished:  workerFinished,
		closeSignal:     closePoller,
		polled:          polled,
		remaining:       0,
		threshold:       int(math.Round(float64(builder.maxJobsActive) * builder.pollThreshold)),
		metrics:         builder.metrics,
		shouldRetry:     builder.shouldRetry,
		backoffSupplier: builder.backoffSupplier,
	}
	if builder.streamEnabled {
		poller.streamNoJobsBackoffSupplier = builder.streamNoJobsBackoff
	}

	dispatcher := jobDispatcher{
		jobQueue:       jobQueue,
		workerFinished: workerFinished,
		closeSignal:    closeDispatcher,
		polled:         polled,
	}

	go dispatcher.run(builder.jobClient, builder.handler, builder.concurrency, &closeWait)
	go poller.poll(&acquireWait)

	if builder.streamEnabled {
		streamRequest := commands.NewStreamJobsCommand(builder.gatewayClient, builder.shouldRetry).
			JobType(builder.request.Type).
			Consumer(jobQueue).
			Timeout(time.Duration(builder.request.Timeout) * time.Millisecond).
			FetchVariables(builder.request.FetchVariable...).
			TenantIds(builder.request.TenantIds...).
			WorkerName(builder.request.Worker).
			InactivityTimeout(builder.effectiveStreamInactivityTimeout()).
			TimeoutJitter(builder.streamTimeoutJitter).
			OnRecycle(builder.streamRecycleCallback()).
			DebugLogger(builder.streamDebugLogger).
			RequestTimeout(builder.streamRequestTimeout)
		streamer := jobStreamer{
			workerFinished:  workerFinished,
			closeSignal:     closeStreamer,
			request:         streamRequest,
			backoffSupplier: builder.backoffSupplier,
			retryDelay:      0,
		}

		go streamer.stream(&acquireWait)
	} else {
		// simulate streamer is already closed
		acquireWait.Done()
	}

	return jobWorkerController{
		closePoller:     closePoller,
		closeDispatcher: closeDispatcher,
		closeStreamer:   closeStreamer,
		acquireWait:     &acquireWait,
		closeWait:       &closeWait,
	}
}

func (builder *JobWorkerBuilder) streamRecycleCallback() func(string) {
	streamMetrics, ok := builder.metrics.(JobWorkerStreamMetrics)
	if !ok {
		return nil
	}
	jobType := builder.request.Type
	return func(reason string) { streamMetrics.IncrementStreamRecycledCount(jobType, reason) }
}

// NewJobWorkerBuilder should use the same retryPredicate used by the CredentialProvider (ShouldRetry method):
//
//	credsProvider, _ := zbc.NewOAuthCredentialsProvider(...)
//	worker.NewJobWorkerBuilder(..., credsProvider.ShouldRetry)
func NewJobWorkerBuilder(gatewayClient pb.GatewayClient, jobClient JobClient, retryPred func(ctx context.Context, err error) bool) JobWorkerBuilderStep1 {
	return &JobWorkerBuilder{
		gatewayClient: gatewayClient,
		jobClient:     jobClient,
		maxJobsActive: DefaultJobWorkerMaxJobActive,
		concurrency:   DefaultJobWorkerConcurrency,
		pollInterval:  DefaultJobWorkerPollInterval,
		pollThreshold: DefaultJobWorkerPollThreshold,
		request: &pb.ActivateJobsRequest{
			Timeout:        commands.DefaultJobTimeoutInMs,
			Worker:         commands.DefaultJobWorkerName,
			RequestTimeout: DefaultRequestTimeout.Milliseconds(),
			TenantIds:      []string{commands.DefaultJobTenantID},
		},
		requestTimeout:  DefaultRequestTimeout + RequestTimeoutOffset,
		shouldRetry:     retryPred,
		backoffSupplier: defaultBackoffSupplier,

		streamNoJobsBackoff:     DefaultStreamNoJobsBackoffSupplier,
		streamRequestTimeout:    DefaultStreamRequestTimeout,
		streamInactivityTimeout: DefaultStreamInactivityTimeout,
		streamTimeoutJitter:     DefaultStreamTimeoutJitter,
	}

}
