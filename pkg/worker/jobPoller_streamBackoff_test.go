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
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/mock_pb"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pollResult struct {
	jobs int
	err  error
}

func newStreamBackoffPoller(t *testing.T, results []pollResult, noJobs BackoffSupplier) *jobPoller {
	ctrl := gomock.NewController(t)
	client := mock_pb.NewMockGatewayClient(ctrl)
	for _, r := range results {
		stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
		if r.err != nil {
			stream.EXPECT().Recv().Return(nil, r.err)
		} else {
			jobs := make([]*pb.ActivatedJob, r.jobs)
			for i := range jobs {
				jobs[i] = &pb.ActivatedJob{Key: int64(i)}
			}
			if r.jobs > 0 {
				stream.EXPECT().Recv().Return(&pb.ActivateJobsResponse{Jobs: jobs}, nil)
			}
			stream.EXPECT().Recv().Return(nil, io.EOF)
		}
		client.EXPECT().ActivateJobs(gomock.Any(), gomock.Any()).Return(stream, nil)
	}
	return &jobPoller{
		client:                      client,
		request:                     &pb.ActivateJobsRequest{},
		requestTimeout:              time.Second,
		maxJobsActive:               1000,
		pollInterval:                100 * time.Millisecond,
		initialPollInterval:         100 * time.Millisecond,
		jobQueue:                    make(chan entities.Job, 1000),
		shouldRetry:                 func(context.Context, error) bool { return false },
		backoffSupplier:             NewExponentialBackoffBuilder().Random(rand.New(rand.NewSource(1))).Build(),
		streamNoJobsBackoffSupplier: noJobs,
	}
}

func noJitterStreamBackoff() BackoffSupplier {
	return NewExponentialBackoffBuilder().MaxDelay(time.Minute).JitterFactor(0).Build()
}

func TestStreamNoJobsBackoffGrowsAndCaps(t *testing.T) {
	results := make([]pollResult, 20)
	poller := newStreamBackoffPoller(t, results, noJitterStreamBackoff())

	prev := poller.pollInterval
	for i := 0; i < 20; i++ {
		poller.activateJobs()
		require.GreaterOrEqual(t, poller.pollInterval, prev)
		require.LessOrEqual(t, poller.pollInterval, time.Minute)
		prev = poller.pollInterval
	}
	require.Equal(t, time.Minute, poller.pollInterval)
}

func TestStreamNoJobsBackoffFirstDelays(t *testing.T) {
	poller := newStreamBackoffPoller(t, make([]pollResult, 3), noJitterStreamBackoff())
	expected := []time.Duration{160 * time.Millisecond, 256 * time.Millisecond, 410 * time.Millisecond}
	for _, e := range expected {
		poller.activateJobs()
		require.Equal(t, e, poller.pollInterval)
	}
}

func TestStreamNoJobsBackoffResetsOnJobs(t *testing.T) {
	poller := newStreamBackoffPoller(t, []pollResult{{}, {}, {jobs: 2}}, noJitterStreamBackoff())
	poller.activateJobs()
	poller.activateJobs()
	require.Greater(t, poller.pollInterval, 100*time.Millisecond)
	poller.activateJobs()
	require.Equal(t, 100*time.Millisecond, poller.pollInterval)
	require.Equal(t, 2, poller.remaining)
}

func TestStreamNoJobsBackoffDefaultJitterWithinBounds(t *testing.T) {
	poller := newStreamBackoffPoller(t, make([]pollResult, 30), DefaultStreamNoJobsBackoffSupplier)
	for i := 0; i < 30; i++ {
		poller.activateJobs()
	}
	require.LessOrEqual(t, poller.pollInterval, 60*time.Second)
	require.GreaterOrEqual(t, poller.pollInterval, 54*time.Second)
}

func TestStreamNoJobsBackoffNotAppliedWhenReopenFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock_pb.NewMockGatewayClient(ctrl)
	// First stream: Recv fails with a retryable error, so the poller tries to reopen the stream.
	failing := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
	failing.EXPECT().Recv().Return(nil, status.Error(codes.Unauthenticated, "token expired"))
	client.EXPECT().ActivateJobs(gomock.Any(), gomock.Any()).Return(failing, nil)
	// The reopen fails: the poll must be marked failed, so the empty-poll backoff must not run.
	client.EXPECT().ActivateJobs(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.Unauthenticated, "still unauthorized")).AnyTimes()

	// Retryable on the Recv error so the reopen is attempted; not retryable on the reopen error
	// itself, so openStream does not recurse forever.
	retries := 0
	poller := &jobPoller{
		client:              client,
		request:             &pb.ActivateJobsRequest{},
		requestTimeout:      time.Second,
		maxJobsActive:       1000,
		pollInterval:        100 * time.Millisecond,
		initialPollInterval: 100 * time.Millisecond,
		jobQueue:            make(chan entities.Job, 1000),
		shouldRetry: func(context.Context, error) bool {
			retries++
			return retries == 1
		},
		backoffSupplier:             NewExponentialBackoffBuilder().Random(rand.New(rand.NewSource(1))).Build(),
		streamNoJobsBackoffSupplier: noJitterStreamBackoff(),
	}

	poller.activateJobs()
	require.Equal(t, 100*time.Millisecond, poller.pollInterval)
}

func TestNoStreamBackoffWhenDisabled(t *testing.T) {
	poller := newStreamBackoffPoller(t, make([]pollResult, 3), nil)
	for i := 0; i < 3; i++ {
		poller.activateJobs()
		require.Equal(t, 100*time.Millisecond, poller.pollInterval)
	}
}

func TestErrorBackoffUnaffectedByStreamBackoff(t *testing.T) {
	errBackoff := NewExponentialBackoffBuilder().JitterFactor(0).Build()
	poller := newStreamBackoffPoller(t, []pollResult{{err: status.Error(codes.ResourceExhausted, "busy")}}, noJitterStreamBackoff())
	poller.backoffSupplier = errBackoff
	poller.activateJobs()
	// error backoff applied once (×1.6 under the 5s cap), stream backoff not applied on top
	require.Equal(t, 160*time.Millisecond, poller.pollInterval)
}

func TestBuilderWiresStreamNoJobsBackoff(t *testing.T) {
	b := NewJobWorkerBuilder(nil, nil, nil).(*JobWorkerBuilder)
	require.NotNil(t, b.streamNoJobsBackoff)
	require.Equal(t, 60*time.Second, b.streamNoJobsBackoff.SupplyRetryDelay(60*time.Second))
	b.StreamNoJobsBackoffSupplier(nil)
	require.Nil(t, b.streamNoJobsBackoff)
}
