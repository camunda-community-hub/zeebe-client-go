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
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/camunda/zeebe/clients/go/v8/internal/mock_pb"
	"github.com/camunda/zeebe/clients/go/v8/internal/utils"
	"github.com/camunda/zeebe/clients/go/v8/pkg/entities"
	"github.com/camunda/zeebe/clients/go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStreamJobsCommand(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)

	request := &pb.StreamActivatedJobsRequest{
		Type:    "foo",
		Timeout: DefaultJobTimeoutInMs,
		Worker:  DefaultJobWorkerName,
	}

	job1 := pb.ActivatedJob{
		Key:                      123,
		Type:                     "foo",
		Retries:                  3,
		Deadline:                 123123,
		Worker:                   DefaultJobWorkerName,
		ElementInstanceKey:       123,
		ProcessDefinitionKey:     124,
		BpmnProcessId:            "fooProcess",
		ProcessInstanceKey:       1233,
		ElementId:                "foobar",
		ProcessDefinitionVersion: 12345,
		CustomHeaders:            "{\"foo\": \"bar\"}",
		Variables:                "{\"foo\": \"bar\"}",
	}
	job2 := pb.ActivatedJob{
		Key:           123,
		Type:          "foo",
		Retries:       3,
		Deadline:      123123,
		Worker:        DefaultJobWorkerName,
		CustomHeaders: "{}",
		Variables:     "{}",
	}
	expectedJobs := []entities.Job{{ActivatedJob: &job1}, {ActivatedJob: &job2}}

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	jobsChan := make(chan entities.Job, 2)
	defer close(jobsChan)

	// when - then
	// setting up the expectations is in itself asserting - any calls with different arguments will cause the
	// test to fail
	gomock.InOrder(
		stream.EXPECT().Recv().Return(&job1, nil),
		stream.EXPECT().Recv().Return(&job2, nil),
		stream.EXPECT().Recv().Return(nil, io.EOF),
	)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stream, nil)
	err := NewStreamJobsCommand(client, func(context.Context, error) bool {
		return false
	}).JobType("foo").Consumer(jobsChan).Send(ctx)

	assert.NoError(t, err)

	// simulate job receive
	jobs := []entities.Job{<-jobsChan, <-jobsChan}
	if len(jobs) != len(expectedJobs) {
		t.Error("Failed to receive all jobs: ", jobs, expectedJobs)
	}

	for i, job := range jobs {
		if !reflect.DeepEqual(job, expectedJobs[i]) {
			t.Error("Failed to receive job: ", job, expectedJobs[i])
		}
	}

}

func TestStreamJobsCommandWithTimeout(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)

	request := &pb.StreamActivatedJobsRequest{
		Type:    "foo",
		Timeout: 60 * 1000,
		Worker:  DefaultJobWorkerName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	jobsChan := make(chan entities.Job, 5)
	defer close(jobsChan)

	// when - then
	// setting up the expectations is in itself asserting - any calls with different arguments will cause the
	// test to fail
	stream.EXPECT().Recv().Return(nil, io.EOF)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stream, nil)
	err := NewStreamJobsCommand(client, func(context.Context, error) bool {
		return false
	}).JobType("foo").Consumer(jobsChan).Timeout(1 * time.Minute).Send(ctx)

	if err != nil {
		assert.NoError(t, err, "Failed to send request")
	}
}

func TestStreamJobsCommandWithWorkerName(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)

	request := &pb.StreamActivatedJobsRequest{
		Type:    "foo",
		Timeout: 300 * 1000,
		Worker:  "bar",
	}

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	jobsChan := make(chan entities.Job, 5)
	defer close(jobsChan)

	// when - then
	// setting up the expectations is in itself asserting - any calls with different arguments will cause the
	// test to fail
	stream.EXPECT().Recv().Return(nil, io.EOF)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stream, nil)
	err := NewStreamJobsCommand(client, func(context.Context, error) bool {
		return false
	}).JobType("foo").Consumer(jobsChan).WorkerName("bar").Send(ctx)

	if err != nil {
		assert.NoError(t, err, "Failed to send request")
	}
}

func TestStreamJobsCommandWithFetchVariables(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)

	fetchVariables := []string{"foo", "bar", "baz"}
	request := &pb.StreamActivatedJobsRequest{
		Type:          "foo",
		Timeout:       300 * 1000,
		Worker:        DefaultJobWorkerName,
		FetchVariable: fetchVariables,
	}

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	jobsChan := make(chan entities.Job, 5)
	defer close(jobsChan)

	// when - then
	// setting up the expectations is in itself asserting - any calls with different arguments will cause the
	// test to fail
	stream.EXPECT().Recv().Return(nil, io.EOF)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stream, nil)
	err := NewStreamJobsCommand(client, func(context.Context, error) bool {
		return false
	}).JobType("foo").Consumer(jobsChan).FetchVariables(fetchVariables...).Send(ctx)

	if err != nil {
		assert.NoError(t, err, "Failed to send request")
	}
}

func TestStreamJobsCommandWithTenantIds(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)
	stream := mock_pb.NewMockGateway_StreamActivatedJobsClient(ctrl)

	tenantIds := []string{"1234", "5555"}
	request := &pb.StreamActivatedJobsRequest{
		Type:      "foo",
		Timeout:   300 * 1000,
		Worker:    DefaultJobWorkerName,
		TenantIds: tenantIds,
	}
	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	jobsChan := make(chan entities.Job, 5)
	defer close(jobsChan)

	// when - then
	// setting up the expectations is in itself asserting - any calls with different arguments will cause the
	// test to fail
	stream.EXPECT().Recv().Return(nil, io.EOF)
	client.EXPECT().StreamActivatedJobs(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stream, nil)
	err := NewStreamJobsCommand(client, func(context.Context, error) bool {
		return false
	}).JobType("foo").Consumer(jobsChan).TenantIds(tenantIds...).Send(ctx)

	if err != nil {
		assert.NoError(t, err, "Failed to send request")
	}
}
