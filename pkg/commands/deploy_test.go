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
	"github.com/camunda-cloud/zeebe/clients/go/internal/mock_pb"
	"github.com/camunda-cloud/zeebe/clients/go/internal/utils"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/pb"
	"github.com/golang/mock/gomock"
	"io/ioutil"
	"testing"
)

func TestDeployCommand_AddResourceFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	demoName := "../../../java/src/test/resources/processes/demo-process.bpmn"
	demoBytes := readBytes(t, demoName)
	anotherName := "../../../java/src/test/resources/processes/another-demo-process.bpmn"
	anotherBytes := readBytes(t, anotherName)

	request := &pb.DeployProcessRequest{
		Processes: []*pb.ProcessRequestObject{
			{
				Name:       demoName,
				Type:       pb.ProcessRequestObject_FILE,
				Definition: demoBytes,
			},
			{
				Name:       anotherName,
				Type:       pb.ProcessRequestObject_FILE,
				Definition: anotherBytes,
			},
		},
	}
	stub := &pb.DeployProcessResponse{}

	client.EXPECT().DeployProcess(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewDeployCommand(client, func(context.Context, error) bool { return false })

	response, err := command.
		AddResourceFile(demoName).
		AddResourceFile(anotherName).
		Send(context.Background())

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestDeployCommand_AddResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	demoName := "../../../java/src/test/resources/processes/demo-process.bpmn"
	demoBytes := readBytes(t, demoName)

	request := &pb.DeployProcessRequest{
		Processes: []*pb.ProcessRequestObject{
			{
				Name:       demoName,
				Type:       pb.ProcessRequestObject_BPMN,
				Definition: demoBytes,
			},
		},
	}
	stub := &pb.DeployProcessResponse{}

	client.EXPECT().DeployProcess(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewDeployCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	response, err := command.
		AddResource(demoBytes, demoName, pb.ProcessRequestObject_BPMN).
		Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func readBytes(t *testing.T, filename string) []byte {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Error("Failed to read file ", err)
	}

	return bytes
}
