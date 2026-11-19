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
	"errors"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

type MigrateInstanceCommandStep1 interface {
	ProcessInstanceKey(int64) MigrateInstanceCommandStep2
}

type MigrateInstanceCommandStep2 interface {
	// MigrationPlan sets the key of the process definition to migrate the instance to.
	MigrationPlan(targetProcessDefinitionKey int64) DispatchMigrateInstanceCommand
	// Plan uses a reusable migration plan built with NewMigrationPlan. The plan is copied,
	// so later changes to the plan, or mapping instructions added to this command, never
	// affect the plan or any other command that uses it.
	Plan(plan *MigrationPlan) DispatchMigrateInstanceCommand
}

// MigrationPlan is a reusable migration plan that can be passed to several migrate
// instance commands (see MigrateInstanceCommandStep2.Plan). It is not safe for
// concurrent modification, but commands only read it when Plan is called.
type MigrationPlan struct {
	targetProcessDefinitionKey int64
	mappingInstructions        []*pb.MigrateProcessInstanceRequest_MappingInstruction
}

// NewMigrationPlan creates a migration plan targeting the given process definition.
func NewMigrationPlan(targetProcessDefinitionKey int64) *MigrationPlan {
	return &MigrationPlan{targetProcessDefinitionKey: targetProcessDefinitionKey}
}

// AddMappingInstruction maps an element of the source process to an element of the
// target process and returns the plan for chaining.
func (p *MigrationPlan) AddMappingInstruction(sourceElementID string, targetElementID string) *MigrationPlan {
	p.mappingInstructions = append(p.mappingInstructions, &pb.MigrateProcessInstanceRequest_MappingInstruction{
		SourceElementId: sourceElementID,
		TargetElementId: targetElementID,
	})
	return p
}

// toProto returns a deep copy of the plan as a request message.
func (p *MigrationPlan) toProto() *pb.MigrateProcessInstanceRequest_MigrationPlan {
	instructions := make([]*pb.MigrateProcessInstanceRequest_MappingInstruction, 0, len(p.mappingInstructions))
	for _, m := range p.mappingInstructions {
		instructions = append(instructions, &pb.MigrateProcessInstanceRequest_MappingInstruction{
			SourceElementId: m.GetSourceElementId(),
			TargetElementId: m.GetTargetElementId(),
		})
	}
	return &pb.MigrateProcessInstanceRequest_MigrationPlan{
		TargetProcessDefinitionKey: p.targetProcessDefinitionKey,
		MappingInstructions:        instructions,
	}
}

var errNilMigrationPlan = errors.New("migration plan must not be nil")

type DispatchMigrateInstanceCommand interface {
	// AddMappingInstruction maps an element of the source process to an element of the
	// target process.
	AddMappingInstruction(sourceElementID string, targetElementID string) DispatchMigrateInstanceCommand
	Send(context.Context) (*pb.MigrateProcessInstanceResponse, error)
}

type MigrateInstanceCommand struct {
	Command
	request pb.MigrateProcessInstanceRequest
	err     error
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

func (cmd *MigrateInstanceCommand) Plan(plan *MigrationPlan) DispatchMigrateInstanceCommand {
	if plan == nil {
		cmd.err = errNilMigrationPlan
		cmd.request.MigrationPlan = &pb.MigrateProcessInstanceRequest_MigrationPlan{}
		return cmd
	}
	cmd.request.MigrationPlan = plan.toProto()
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
	if cmd.err != nil {
		return nil, cmd.err
	}
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
