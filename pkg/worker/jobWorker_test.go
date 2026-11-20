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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/mock_pb"
	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/commands"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/entities"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// rpcMsg implements the gomock.Matcher interface
type rpcMsg struct {
	msg proto.Message
}

func (r *rpcMsg) Matches(msg interface{}) bool {
	m, ok := msg.(proto.Message)
	if !ok {
		return false
	}
	return proto.Equal(m, r.msg)
}

func (r *rpcMsg) String() string {
	return fmt.Sprintf("is %s", r.msg)
}

func TestJobWorkerActivateJobsDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)

	request := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: DefaultJobWorkerMaxJobActive,
		Timeout:           commands.DefaultJobTimeoutInMs,
		Worker:            commands.DefaultJobWorkerName,
		RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	response := &pb.ActivateJobsResponse{
		Jobs: []*pb.ActivatedJob{
			{
				Key: 1,
			},
		},
	}

	stream.EXPECT().Recv().Return(response, nil).AnyTimes()

	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: request}).Return(stream, nil)

	jobs := make(chan entities.Job, 1)

	NewJobWorkerBuilder(client, nil, nil).JobType("foo").Handler(func(client JobClient, job entities.Job) {
		jobs <- job
	}).RequestTimeout(utils.DefaultTestTimeout).Open()

	select {
	case job := <-jobs:
		expected := response.Jobs[0].Key
		if job.Key != expected {
			t.Error("Failed to received job", expected, "got", job.Key)
		}
	case <-time.After(utils.DefaultTestTimeout):
		t.Error("Failed to receive all jobs before timeout")
	}
}

func TestJobWorkerActivateJobsCustom(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)

	timeout := 7 * time.Minute

	request := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: 123,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "fooWorker",
		RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	response := &pb.ActivateJobsResponse{
		Jobs: []*pb.ActivatedJob{
			{
				Key: 1,
			},
		},
	}

	stream.EXPECT().Recv().Return(response, nil).AnyTimes()

	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: request}).Return(stream, nil)

	jobs := make(chan entities.Job, 1)

	NewJobWorkerBuilder(client, nil, nil).JobType("foo").Handler(func(client JobClient, job entities.Job) {
		jobs <- job
	}).MaxJobsActive(123).Timeout(timeout).RequestTimeout(utils.DefaultTestTimeout).Name("fooWorker").Open()

	select {
	case job := <-jobs:
		expected := response.Jobs[0].Key
		if job.Key != expected {
			t.Error("Failed to received job", expected, "got", job.Key)
		}
	case <-time.After(utils.DefaultTestTimeout):
		t.Error("Failed to receive all jobs before timeout")
	}
}

func TestJobWorkerStreamDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)

	timeout := 7 * time.Minute

	request := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: 123,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "fooWorker",
		RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	response := &pb.ActivateJobsResponse{
		Jobs: []*pb.ActivatedJob{
			{
				Key: 1,
			},
		},
	}

	stream.EXPECT().Recv().Return(response, nil).AnyTimes()
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: request}).Return(stream, nil)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), gomock.Any()).Times(0)

	jobs := make(chan entities.Job, 1)
	NewJobWorkerBuilder(client, nil, nil).JobType("foo").Handler(func(client JobClient, job entities.Job) {
		jobs <- job
	}).MaxJobsActive(123).Timeout(timeout).RequestTimeout(utils.DefaultTestTimeout).Name("fooWorker").Open()

	select {
	case job := <-jobs:
		expected := response.Jobs[0].Key
		if job.Key != expected {
			t.Error("Failed to received job", expected, "got", job.Key)
		}
	case <-time.After(utils.DefaultTestTimeout):
		t.Error("Failed to receive all jobs before timeout")
	}
}

func TestJobWorkerStreamEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)
	activateJobsStream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
	timeout := 7 * time.Minute
	request := &pb.StreamActivatedJobsRequest{
		Type:      "foo",
		Timeout:   int64(timeout / time.Millisecond),
		Worker:    "fooWorker",
		TenantIds: []string{commands.DefaultJobTenantID},
	}
	activateJobsRequest := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: 123,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "fooWorker",
		RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	response := &pb.ActivatedJob{Key: 1}
	emptyActivateResponse := &pb.ActivateJobsResponse{}

	stream.EXPECT().Recv().Return(response, nil).AnyTimes()
	activateJobsStream.EXPECT().Recv().Return(emptyActivateResponse, nil).AnyTimes()
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &rpcMsg{msg: request}).AnyTimes().Return(stream, nil)
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest}).Return(activateJobsStream, nil).AnyTimes()

	retryPred := func(ctx context.Context, err error) bool { return true }
	jobs := make(chan entities.Job, 1)
	NewJobWorkerBuilder(client, nil, retryPred).
		JobType("foo").
		Handler(func(client JobClient, job entities.Job) {
			jobs <- job
		}).
		MaxJobsActive(123).
		Timeout(timeout).
		RequestTimeout(utils.DefaultTestTimeout).
		Name("fooWorker").
		StreamEnabled(true).
		PollInterval(time.Minute).
		Open()

	select {
	case job := <-jobs:
		expected := response.Key
		if job.Key != expected {
			t.Error("Failed to received job", expected, "got", job.Key)
		}
	case <-time.After(utils.DefaultTestTimeout):
		t.Error("Failed to receive all jobs before timeout")
	}
}

func TestStreamingJobWorkerClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)
	activateJobsStream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
	timeout := 7 * time.Minute
	request := &pb.StreamActivatedJobsRequest{
		Type:      "foo",
		Timeout:   int64(timeout / time.Millisecond),
		Worker:    "default",
		TenantIds: []string{commands.DefaultJobTenantID},
	}
	activateJobsRequest := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: 32,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "default",
		RequestTimeout:    10000,
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	stream.EXPECT().Recv().Return(nil, io.EOF).AnyTimes()
	activateJobsStream.EXPECT().Recv().Return(nil, io.EOF).AnyTimes()
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &rpcMsg{msg: request}).AnyTimes().Return(stream, nil)
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest}).Return(activateJobsStream, nil).AnyTimes()

	retryPred := func(ctx context.Context, err error) bool { return err != io.EOF }
	worker := NewJobWorkerBuilder(client, nil, retryPred).
		JobType("foo").
		Handler(func(client JobClient, job entities.Job) {}).
		Timeout(timeout).
		StreamEnabled(true).
		Open()

	closedChan := make(chan bool)
	go func() {
		worker.Close()
		closedChan <- true
		close(closedChan)
	}()

	select {
	case closed := <-closedChan:
		assert.True(t, closed, "Worker should eventually close")
		break
	case <-time.After(10 * time.Second):
		assert.Fail(t, "Failed to close worker after 10 seconds")
		break
	}
}

// Close must wait for a streaming Send that was canceled but has not returned yet: every job the
// stream already received has to reach the handler before Close completes. The mock stream delivers
// one job, then holds Send open after cancellation until the test releases it; Close must stay
// blocked while Send is held, and the streamed job must be handled once Send returns.
func TestStreamingJobWorkerCloseWaitsForCanceledStreamSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)
	timeout := 7 * time.Minute

	streamRequest := &pb.StreamActivatedJobsRequest{
		Type:      "foo",
		Timeout:   int64(timeout / time.Millisecond),
		Worker:    "fooWorker",
		TenantIds: []string{commands.DefaultJobTenantID},
	}
	activateJobsRequest := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: DefaultJobWorkerMaxJobActive,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "fooWorker",
		RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	// no polled jobs, so only the streamed job is in play; the long poll blocks until Close
	// cancels it, so the poller exits instead of spinning on instant empty responses
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest}).DoAndReturn(
		func(ctx context.Context, _ *pb.ActivateJobsRequest, _ ...grpc.CallOption) (pb.Gateway_ActivateJobsClient, error) {
			s := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
			s.EXPECT().Recv().DoAndReturn(func() (*pb.ActivateJobsResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}).AnyTimes()
			return s, nil
		}).AnyTimes()

	var streamCtx atomic.Value
	jobReceived := make(chan struct{})
	releaseSend := make(chan struct{})
	var recvCount int32
	stream.EXPECT().Recv().DoAndReturn(func() (*pb.ActivatedJob, error) {
		ctx := streamCtx.Load().(context.Context)
		if atomic.AddInt32(&recvCount, 1) == 1 {
			// first Recv delivers the already-received job
			close(jobReceived)
			return &pb.ActivatedJob{Key: 1}, nil
		}
		// subsequent Recv blocks until Close cancels the stream, then stays open
		// (canceled but not finished) until the test releases it
		<-ctx.Done()
		<-releaseSend
		return nil, ctx.Err()
	}).AnyTimes()
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &rpcMsg{msg: streamRequest}).AnyTimes().DoAndReturn(
		func(ctx context.Context, _ *pb.StreamActivatedJobsRequest, _ ...grpc.CallOption) (pb.Gateway_StreamActivatedJobsClient, error) {
			streamCtx.Store(ctx)
			return stream, nil
		})

	retryPred := func(ctx context.Context, err error) bool { return false }
	handled := make(chan int64, 1)
	worker := NewJobWorkerBuilder(client, nil, retryPred).
		JobType("foo").
		Handler(func(_ JobClient, job entities.Job) {
			handled <- job.Key
		}).
		Timeout(timeout).
		RequestTimeout(utils.DefaultTestTimeout).
		Name("fooWorker").
		StreamEnabled(true).
		Open()

	// the stream is open and has delivered its job to the consumer before we close
	select {
	case <-jobReceived:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("stream did not deliver its job before timeout")
	}

	closed := make(chan struct{})
	go func() {
		worker.Close()
		close(closed)
	}()

	// Close cancels the stream, but Send is still held open: Close must not complete
	select {
	case <-closed:
		t.Fatal("Close completed while the canceled streaming Send was still open")
	case <-time.After(200 * time.Millisecond):
	}

	// let the canceled Send return; the already-received job must be handled before Close completes
	close(releaseSend)
	select {
	case key := <-handled:
		if key != 1 {
			t.Fatalf("handled wrong job: got %d, want 1", key)
		}
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("streamed job was not handled after Send returned")
	}

	select {
	case <-closed:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("Close did not complete after the streaming Send returned")
	}
}

func TestJobWorkerClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
	timeout := 7 * time.Minute
	request := &pb.ActivateJobsRequest{
		Type:              "foo",
		MaxJobsToActivate: 32,
		Timeout:           int64(timeout / time.Millisecond),
		Worker:            "default",
		RequestTimeout:    10000,
		TenantIds:         []string{commands.DefaultJobTenantID},
	}

	stream.EXPECT().Recv().Return(nil, io.EOF).AnyTimes()
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: request}).Return(stream, nil).AnyTimes()

	retryPred := func(ctx context.Context, err error) bool { return err != io.EOF }
	worker := NewJobWorkerBuilder(client, nil, retryPred).
		JobType("foo").
		Handler(func(client JobClient, job entities.Job) {}).
		Timeout(timeout).
		StreamEnabled(false).
		Open()

	closedChan := make(chan bool)
	go func() {
		worker.Close()
		closedChan <- true
		close(closedChan)
	}()

	select {
	case closed := <-closedChan:
		assert.True(t, closed, "Worker should eventually close")
		break
	case <-time.After(10 * time.Second):
		assert.Fail(t, "Failed to close worker after 10 seconds")
		break
	}
}

func TestJobWorkerStreamEnabledPollsAgainAfterEachPolledBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)
	timeout := 7 * time.Minute

	streamRequest := &pb.StreamActivatedJobsRequest{
		Type:      "foo",
		Timeout:   int64(timeout / time.Millisecond),
		Worker:    "fooWorker",
		TenantIds: []string{commands.DefaultJobTenantID},
	}
	activateJobsRequest := func(maxJobsToActivate int32) *pb.ActivateJobsRequest {
		return &pb.ActivateJobsRequest{
			Type:              "foo",
			MaxJobsToActivate: maxJobsToActivate,
			Timeout:           int64(timeout / time.Millisecond),
			Worker:            "fooWorker",
			RequestTimeout:    int64(utils.DefaultTestTimeout / time.Millisecond),
			TenantIds:         []string{commands.DefaultJobTenantID},
		}
	}

	// the streaming RPC stays open for the whole test without delivering jobs, until
	// worker.Close() cancels the RPC context
	var streamCtx atomic.Value
	stream.EXPECT().Recv().DoAndReturn(func() (*pb.ActivatedJob, error) {
		ctx := streamCtx.Load().(context.Context)
		<-ctx.Done()
		return nil, ctx.Err()
	}).AnyTimes()
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &rpcMsg{msg: streamRequest}).AnyTimes().DoAndReturn(
		func(ctx context.Context, _ *pb.StreamActivatedJobsRequest, _ ...grpc.CallOption) (pb.Gateway_StreamActivatedJobsClient, error) {
			streamCtx.Store(ctx)
			return stream, nil
		})

	batch := func(keys ...int64) *mock_pb.MockGateway_ActivateJobsClient {
		jobs := make([]*pb.ActivatedJob, len(keys))
		for i, key := range keys {
			jobs[i] = &pb.ActivatedJob{Key: key}
		}
		s := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
		gomock.InOrder(
			s.EXPECT().Recv().Return(&pb.ActivateJobsResponse{Jobs: jobs}, nil),
			s.EXPECT().Recv().Return(nil, io.EOF),
		)
		return s
	}

	secondPoll := make(chan struct{})
	gomock.InOrder(
		client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest(2)}).Return(batch(1, 2), nil),
		client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest(2)}).DoAndReturn(
			func(context.Context, *pb.ActivateJobsRequest, ...grpc.CallOption) (pb.Gateway_ActivateJobsClient, error) {
				close(secondPoll)
				return batch(3, 4), nil
			}),
	)
	// finishing the second batch can race the deferred Close and trigger one more poll;
	// it must find no jobs so the worker can close cleanly
	empty := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
	empty.EXPECT().Recv().Return(nil, io.EOF).AnyTimes()
	client.EXPECT().ActivateJobs(gomock.Any(), &rpcMsg{msg: activateJobsRequest(2)}).Return(empty, nil).AnyTimes()

	retryPred := func(ctx context.Context, err error) bool { return false }
	activate := make(chan entities.Job)
	release := make(chan struct{}, 4)
	worker := NewJobWorkerBuilder(client, nil, retryPred).
		JobType("foo").
		Handler(func(client JobClient, job entities.Job) {
			activate <- job
			<-release
		}).
		MaxJobsActive(2).
		Concurrency(2).
		Timeout(timeout).
		RequestTimeout(utils.DefaultTestTimeout).
		Name("fooWorker").
		StreamEnabled(true).
		PollInterval(time.Hour).
		PollThreshold(0.1).
		Open()
	defer func() {
		closed := make(chan struct{})
		go func() {
			worker.Close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(utils.DefaultTestTimeout):
			t.Error("Failed to close worker before timeout")
		}
	}()

	// the full first batch is activated while the streaming RPC stays open
	for i := 0; i < 2; i++ {
		select {
		case <-activate:
		case <-time.After(utils.DefaultTestTimeout):
			t.Fatal("Failed to activate first polled batch before timeout")
		}
	}

	// completing only part of the batch must not poll again
	release <- struct{}{}
	select {
	case <-secondPoll:
		t.Fatal("Polled again before the full polled batch finished")
	case <-time.After(500 * time.Millisecond):
	}

	// finishing the last job of the batch resumes polling even though the stream stays open
	release <- struct{}{}
	select {
	case <-secondPoll:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("Poller did not poll again after the polled batch finished with the stream open")
	}

	// drain the second batch so the worker can close
	for i := 0; i < 2; i++ {
		select {
		case <-activate:
		case <-time.After(utils.DefaultTestTimeout):
			t.Fatal("Failed to activate second polled batch before timeout")
		}
	}
	release <- struct{}{}
	release <- struct{}{}
}

// Jobs activated by an in-flight poll while the worker closes must be handed to the handler,
// never stranded until their timeout; Close must also cancel the in-flight long poll.
func TestJobWorkerCloseHandlesJobsActivatedDuringClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	polling := make(chan struct{})
	var once sync.Once
	client.EXPECT().ActivateJobs(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *pb.ActivateJobsRequest, _ ...grpc.CallOption) (pb.Gateway_ActivateJobsClient, error) {
			stream := mock_pb.NewMockGateway_ActivateJobsClient(ctrl)
			delivered := false
			stream.EXPECT().Recv().DoAndReturn(func() (*pb.ActivateJobsResponse, error) {
				once.Do(func() { close(polling) })
				// the long poll only returns once Close cancels it; the gateway answers with jobs
				// it activated just before the cancellation reached it
				<-ctx.Done()
				if !delivered {
					delivered = true
					return &pb.ActivateJobsResponse{Jobs: []*pb.ActivatedJob{{Key: 1}, {Key: 2}, {Key: 3}}}, nil
				}
				return nil, status.Error(codes.Canceled, ctx.Err().Error())
			}).AnyTimes()
			return stream, nil
		}).AnyTimes()

	var mu sync.Mutex
	handled := map[int64]bool{}
	worker := NewJobWorkerBuilder(client, nil, func(context.Context, error) bool { return false }).
		JobType("foo").
		Handler(func(_ JobClient, job entities.Job) {
			mu.Lock()
			handled[job.Key] = true
			mu.Unlock()
		}).
		RequestTimeout(time.Hour).
		StreamEnabled(false).
		Open()

	select {
	case <-polling:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("worker did not start polling")
	}

	closed := make(chan struct{})
	go func() {
		worker.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(utils.DefaultTestTimeout):
		t.Fatal("Close did not cancel the in-flight poll")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, map[int64]bool{1: true, 2: true, 3: true}, handled, "all activated jobs must be handled on close")
}
