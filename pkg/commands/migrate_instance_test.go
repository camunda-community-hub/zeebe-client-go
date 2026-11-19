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

func TestMigrateInstanceCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	request := &pb.MigrateProcessInstanceRequest{
		ProcessInstanceKey: 123,
		MigrationPlan: &pb.MigrateProcessInstanceRequest_MigrationPlan{
			TargetProcessDefinitionKey: 456,
			MappingInstructions: []*pb.MigrateProcessInstanceRequest_MappingInstruction{
				{SourceElementId: "taskA", TargetElementId: "taskA2"},
				{SourceElementId: "taskB", TargetElementId: "taskB2"},
			},
		},
	}
	stub := &pb.MigrateProcessInstanceResponse{}

	client.EXPECT().MigrateProcessInstance(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	response, err := NewMigrateInstanceCommand(client, noRetry).
		ProcessInstanceKey(123).
		MigrationPlan(456).
		AddMappingInstruction("taskA", "taskA2").
		AddMappingInstruction("taskB", "taskB2").
		Send(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestMigrateInstanceCommandWithReusablePlan(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	plan := NewMigrationPlan(456).AddMappingInstruction("taskA", "taskA2")

	first := &pb.MigrateProcessInstanceRequest{
		ProcessInstanceKey: 1,
		MigrationPlan: &pb.MigrateProcessInstanceRequest_MigrationPlan{
			TargetProcessDefinitionKey: 456,
			MappingInstructions: []*pb.MigrateProcessInstanceRequest_MappingInstruction{
				{SourceElementId: "taskA", TargetElementId: "taskA2"},
				{SourceElementId: "extra", TargetElementId: "extra2"},
			},
		},
	}
	second := &pb.MigrateProcessInstanceRequest{
		ProcessInstanceKey: 2,
		MigrationPlan: &pb.MigrateProcessInstanceRequest_MigrationPlan{
			TargetProcessDefinitionKey: 456,
			MappingInstructions: []*pb.MigrateProcessInstanceRequest_MappingInstruction{
				{SourceElementId: "taskA", TargetElementId: "taskA2"},
			},
		},
	}
	stub := &pb.MigrateProcessInstanceResponse{}
	gomock.InOrder(
		client.EXPECT().MigrateProcessInstance(gomock.Any(), &utils.RPCTestMsg{Msg: first}).Return(stub, nil),
		client.EXPECT().MigrateProcessInstance(gomock.Any(), &utils.RPCTestMsg{Msg: second}).Return(stub, nil),
	)

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	// an extra inline instruction on one command must not leak into the plan or other commands
	cmd2 := NewMigrateInstanceCommand(client, noRetry).ProcessInstanceKey(2).Plan(plan)
	if _, err := NewMigrateInstanceCommand(client, noRetry).ProcessInstanceKey(1).Plan(plan).
		AddMappingInstruction("extra", "extra2").Send(ctx); err != nil {
		t.Fatal(err)
	}
	// changing the plan after Plan() was called must not affect that command
	plan.AddMappingInstruction("late", "late2")
	if _, err := cmd2.Send(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(plan.mappingInstructions); got != 2 {
		t.Errorf("expected plan to have 2 instructions, got %d", got)
	}
}

func TestMigrateInstanceCommandWithNilPlan(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	_, err := NewMigrateInstanceCommand(client, noRetry).ProcessInstanceKey(1).Plan(nil).
		AddMappingInstruction("a", "b").Send(context.Background())
	if err == nil {
		t.Fatal("expected error for nil migration plan")
	}
}
