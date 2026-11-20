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
	"sync"
	"testing"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/mock_pb"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// blockingStream returns a gateway whose stream delivers the jobs pushed to the returned channel and
// otherwise blocks until the stream context is done, like a real long-lived gRPC stream.
func blockingStream(t *testing.T) (*mock_pb.MockGatewayClient, chan *pb.ActivatedJob) {
	ctrl := gomock.NewController(t)
	client := mock_pb.NewMockGatewayClient(ctrl)
	jobs := make(chan *pb.ActivatedJob, 100)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, _ *pb.StreamActivatedJobsRequest, _ ...interface{}) (pb.Gateway_StreamActivatedJobsClient, error) {
			stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)
			stream.EXPECT().Recv().AnyTimes().DoAndReturn(func() (*pb.ActivatedJob, error) {
				select {
				case job := <-jobs:
					return job, nil
				case <-ctx.Done():
					return nil, status.FromContextError(ctx.Err()).Err()
				}
			})
			return stream, nil
		})
	return client, jobs
}

type recycles struct {
	mu      sync.Mutex
	reasons []string
}

func (r *recycles) add(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, reason)
}

func (r *recycles) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.reasons...)
}

func TestStreamJobsCommandRecyclesOnMaxLifetimeWithoutLosingJobs(t *testing.T) {
	client, jobs := blockingStream(t)
	consumer := make(chan entities.Job, 10)
	rec := &recycles{}
	jobs <- &pb.ActivatedJob{Key: 1}
	jobs <- &pb.ActivatedJob{Key: 2}

	start := time.Now()
	err := NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(consumer).
		OnRecycle(rec.add).RequestTimeout(100 * time.Millisecond).Send(context.Background())

	require.NoError(t, err, "a lifetime recycle must not be reported as an error")
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
	assert.Equal(t, []string{StreamRecycleReasonLifetime}, rec.get())
	require.Len(t, consumer, 2)
	assert.EqualValues(t, 1, (<-consumer).Key)
	assert.EqualValues(t, 2, (<-consumer).Key)
}

func TestStreamJobsCommandInactivityWatchdogFiresAndResetsOnJob(t *testing.T) {
	client, jobs := blockingStream(t)
	consumer := make(chan entities.Job, 10)
	rec := &recycles{}

	go func() {
		// keep the stream active for longer than the inactivity timeout
		for i := 0; i < 5; i++ {
			time.Sleep(40 * time.Millisecond)
			jobs <- &pb.ActivatedJob{Key: int64(i)}
		}
	}()

	start := time.Now()
	err := NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(consumer).
		InactivityTimeout(100 * time.Millisecond).OnRecycle(rec.add).
		RequestTimeout(time.Hour).Send(context.Background())

	require.NoError(t, err)
	// 5 jobs at ~40ms intervals keep it alive ~200ms, then 100ms of silence
	assert.GreaterOrEqual(t, time.Since(start), 290*time.Millisecond)
	assert.Equal(t, []string{StreamRecycleReasonInactivity}, rec.get())
	assert.Len(t, consumer, 5)
}

func TestStreamJobsCommandBothDisabled(t *testing.T) {
	client, _ := blockingStream(t)
	consumer := make(chan entities.Job, 10)
	rec := &recycles{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(consumer).
		InactivityTimeout(0).OnRecycle(rec.add).RequestTimeout(0).Send(ctx)

	// only the caller's context ends the stream; this is not a recycle
	require.Error(t, err)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	assert.Empty(t, rec.get())
}

func TestStreamJobsCommandCallerCancelIsNotARecycle(t *testing.T) {
	client, _ := blockingStream(t)
	rec := &recycles{}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	err := NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(make(chan entities.Job)).
		InactivityTimeout(time.Hour).OnRecycle(rec.add).RequestTimeout(time.Hour).Send(ctx)

	require.Error(t, err)
	assert.Empty(t, rec.get())
}

func TestStreamJobsCommandDebugLoggerReceivesRecycleMessages(t *testing.T) {
	client, _ := blockingStream(t)
	var mu sync.Mutex
	var msgs []string

	err := NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(make(chan entities.Job, 1)).
		DebugLogger(func(msg string) {
			mu.Lock()
			msgs = append(msgs, msg)
			mu.Unlock()
		}).RequestTimeout(50 * time.Millisecond).Send(context.Background())

	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "foo")
	assert.Contains(t, msgs[0], StreamRecycleReasonLifetime)
}

func TestStreamJobsCommandInactivityWatchdogPausesWhileDeliveryBlocked(t *testing.T) {
	client, jobs := blockingStream(t)
	// unbuffered consumer: delivery of the first job blocks until the test reads it
	consumer := make(chan entities.Job)
	rec := &recycles{}

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- NewStreamJobsCommand(client, noRetry).JobType("foo").Consumer(consumer).
			InactivityTimeout(100 * time.Millisecond).OnRecycle(rec.add).
			RequestTimeout(time.Hour).Send(context.Background())
	}()

	jobs <- &pb.ActivatedJob{Key: 1}

	// keep delivery blocked for longer than the inactivity timeout: the watchdog must not recycle
	// the stream while a job is still waiting to be handed to the consumer
	blocked := 300 * time.Millisecond
	time.Sleep(blocked)
	select {
	case err := <-sendDone:
		t.Fatalf("stream recycled while delivery was blocked: %v", err)
	default:
	}

	// release the consumer: the blocked job is delivered exactly once
	start := time.Now()
	job := <-consumer
	assert.EqualValues(t, 1, job.Key)
	select {
	case extra := <-consumer:
		t.Fatalf("job delivered more than once: %v", extra.Key)
	default:
	}

	// a fresh inactivity interval elapses before the watchdog recycles the stream
	require.NoError(t, <-sendDone)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
	assert.Equal(t, []string{StreamRecycleReasonInactivity}, rec.get())
}

func TestJitterDurationSpreadsAcrossWindow(t *testing.T) {
	d := 8 * time.Hour
	lower := time.Duration(float64(d) * 0.9)
	minSeen, maxSeen := d, time.Duration(0)
	buckets := make(map[int]int)
	for i := 0; i < 1000; i++ {
		j := JitterDuration(d, 0.1)
		require.GreaterOrEqual(t, j, lower)
		require.LessOrEqual(t, j, d)
		if j < minSeen {
			minSeen = j
		}
		if j > maxSeen {
			maxSeen = j
		}
		buckets[int((j-lower)*10/(d-lower+1))]++
	}
	// deadlines are spread over the whole window, not clustered
	assert.Len(t, buckets, 10)
	assert.Greater(t, maxSeen-minSeen, time.Duration(float64(d-lower)*0.8))

	assert.Equal(t, d, JitterDuration(d, 0))
	assert.Equal(t, time.Duration(0), JitterDuration(0, 0.1))
}
