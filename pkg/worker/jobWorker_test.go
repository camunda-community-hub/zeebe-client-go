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

	// the streaming RPC stays open for the whole test without delivering jobs
	stream.EXPECT().Recv().DoAndReturn(func() (*pb.ActivatedJob, error) {
		select {}
	}).AnyTimes()
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &rpcMsg{msg: streamRequest}).AnyTimes().Return(stream, nil)

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
