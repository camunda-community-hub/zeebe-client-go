package worker

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/zeebe-io/zeebe/clients/go/commands"
	"github.com/zeebe-io/zeebe/clients/go/entities"
	"github.com/zeebe-io/zeebe/clients/go/mock_pb"
	"github.com/zeebe-io/zeebe/clients/go/pb"
	"github.com/zeebe-io/zeebe/clients/go/utils"
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
		Type:    "foo",
		Amount:  DefaultJobWorkerBufferSize,
		Timeout: commands.DefaultJobTimeoutInMs,
		Worker:  commands.DefaultJobWorkerName,
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

	NewJobWorkerBuilder(client, nil, utils.DefaultTestTimeout).JobType("foo").Handler(func(client JobClient, job entities.Job) {
		jobs <- job
	}).Open()

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
		Type:    "foo",
		Amount:  123,
		Timeout: int64(timeout / time.Millisecond),
		Worker:  "fooWorker",
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

	NewJobWorkerBuilder(client, nil, utils.DefaultTestTimeout).JobType("foo").Handler(func(client JobClient, job entities.Job) {
		jobs <- job
	}).BufferSize(123).Timeout(timeout).Name("fooWorker").Open()

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
