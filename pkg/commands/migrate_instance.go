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

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

type MigrateInstanceCommandStep1 interface {
	ProcessInstanceKey(int64) MigrateInstanceCommandStep2
}

type MigrateInstanceCommandStep2 interface {
	// MigrationPlan sets the key of the process definition to migrate the instance to.
	MigrationPlan(targetProcessDefinitionKey int64) DispatchMigrateInstanceCommand
}

type DispatchMigrateInstanceCommand interface {
	// AddMappingInstruction maps an element of the source process to an element of the
	// target process.
	AddMappingInstruction(sourceElementID string, targetElementID string) DispatchMigrateInstanceCommand
	Send(context.Context) (*pb.MigrateProcessInstanceResponse, error)
}

type MigrateInstanceCommand struct {
	Command
	request pb.MigrateProcessInstanceRequest
}

func (cmd *MigrateInstanceCommand) ProcessInstanceKey(key int64) MigrateInstanceCommandStep2 {
	cmd.request.ProcessInstanceKey = key
	return cmd
}

func (cmd *MigrateInstanceCommand) MigrationPlan(targetProcessDefinitionKey int64) DispatchMigrateInstanceCommand {
	cmd.request.MigrationPlan = &pb.MigrateProcessInstanceRequest_MigrationPlan{
		TargetProcessDefinitionKey: targetProcessDefinitionKey,
	}
	return cmd
}

func (cmd *MigrateInstanceCommand) AddMappingInstruction(sourceElementID string, targetElementID string) DispatchMigrateInstanceCommand {
	cmd.request.MigrationPlan.MappingInstructions = append(cmd.request.MigrationPlan.MappingInstructions, &pb.MigrateProcessInstanceRequest_MappingInstruction{
		SourceElementId: sourceElementID,
		TargetElementId: targetElementID,
	})
	return cmd
}

func (cmd *MigrateInstanceCommand) Send(ctx context.Context) (*pb.MigrateProcessInstanceResponse, error) {
	response, err := cmd.gateway.MigrateProcessInstance(ctx, &cmd.request)
	if cmd.shouldRetry(ctx, err) {
		return cmd.Send(ctx)
	}

	return response, err
}

func NewMigrateInstanceCommand(gateway pb.GatewayClient, pred retryPredicate) MigrateInstanceCommandStep1 {
	return &MigrateInstanceCommand{
		Command: Command{
			gateway:     gateway,
			shouldRetry: pred,
		},
	}
}
