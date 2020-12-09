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
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/zeebe-io/zeebe/clients/go/internal/mock_pb"
	"github.com/zeebe-io/zeebe/clients/go/internal/utils"
	"github.com/zeebe-io/zeebe/clients/go/pkg/commands"
	"github.com/zeebe-io/zeebe/clients/go/pkg/entities"
	"github.com/zeebe-io/zeebe/clients/go/pkg/pb"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
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
