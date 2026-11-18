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
	"testing"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/mock_pb"
	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
)

func noRetry(context.Context, error) bool { return false }

func TestModifyInstanceCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	request := &pb.ModifyProcessInstanceRequest{
		ProcessInstanceKey: 123,
		ActivateInstructions: []*pb.ModifyProcessInstanceRequest_ActivateInstruction{
			{
				ElementId: "taskA",
				VariableInstructions: []*pb.ModifyProcessInstanceRequest_VariableInstruction{
					{Variables: `{"foo":"bar"}`},
					{Variables: `{"x":1}`, ScopeId: "subprocess"},
				},
			},
			{ElementId: "taskB", AncestorElementInstanceKey: 456},
		},
		TerminateInstructions: []*pb.ModifyProcessInstanceRequest_TerminateInstruction{
			{ElementInstanceKey: 789},
		},
	}
	stub := &pb.ModifyProcessInstanceResponse{}

	client.EXPECT().ModifyProcessInstance(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	activateA, err := NewModifyInstanceCommand(client, noRetry).
		ProcessInstanceKey(123).
		ActivateElement("taskA").
		VariablesFromString(`{"foo":"bar"}`)
	if err != nil {
		t.Fatal(err)
	}
	activateA, err = activateA.VariablesFromMapInScope("subprocess", map[string]interface{}{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	command := activateA.
		ActivateElementWithAncestor("taskB", 456).
		TerminateElement(789)

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	response, err := command.Send(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestModifyInstanceCommandTerminateOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	request := &pb.ModifyProcessInstanceRequest{
		ProcessInstanceKey: 123,
		TerminateInstructions: []*pb.ModifyProcessInstanceRequest_TerminateInstruction{
			{ElementInstanceKey: 1},
			{ElementInstanceKey: 2},
		},
	}
	stub := &pb.ModifyProcessInstanceResponse{}

	client.EXPECT().ModifyProcessInstance(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	_, err := NewModifyInstanceCommand(client, noRetry).
		ProcessInstanceKey(123).
		TerminateElement(1).
		TerminateElement(2).
		Send(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestModifyInstanceCommandRejectsInvalidVariables(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	_, err := NewModifyInstanceCommand(client, noRetry).
		ProcessInstanceKey(123).
		ActivateElement("taskA").
		VariablesFromString("not json")
	if err == nil {
		t.Error("expected an error for invalid variables")
	}
}
