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

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

type ModifyInstanceCommandStep1 interface {
	ProcessInstanceKey(int64) ModifyInstanceCommandStep2
}

// ModifyInstanceCommandStep2 adds an instruction to the modification. At least one
// instruction is required before the command can be sent.
type ModifyInstanceCommandStep2 interface {
	// ActivateElement activates the element with the given id. The element's flow
	// scope must have exactly one active instance; otherwise use
	// ActivateElementWithAncestor.
	ActivateElement(elementID string) ModifyInstanceActivateStep
	// ActivateElementWithAncestor activates the element with the given id inside the
	// flow scope of the given ancestor element instance.
	ActivateElementWithAncestor(elementID string, ancestorElementInstanceKey int64) ModifyInstanceActivateStep
	// TerminateElement terminates the element instance with the given key.
	TerminateElement(elementInstanceKey int64) DispatchModifyInstanceCommand
}

// ModifyInstanceActivateStep sets variables for the element activated last. Without a
// scope, the variables are created in the scope of the activated element; with a scope,
// in the flow scope with that element id.
type ModifyInstanceActivateStep interface {
	DispatchModifyInstanceCommand

	VariablesFromString(variables string) (ModifyInstanceActivateStep, error)
	VariablesFromMap(variables map[string]interface{}) (ModifyInstanceActivateStep, error)
	VariablesFromObject(variables interface{}) (ModifyInstanceActivateStep, error)
	VariablesFromStringInScope(scopeID string, variables string) (ModifyInstanceActivateStep, error)
	VariablesFromMapInScope(scopeID string, variables map[string]interface{}) (ModifyInstanceActivateStep, error)
	VariablesFromObjectInScope(scopeID string, variables interface{}) (ModifyInstanceActivateStep, error)
}

// DispatchModifyInstanceCommand sends the modification or adds further instructions.
type DispatchModifyInstanceCommand interface {
	ModifyInstanceCommandStep2
	Send(context.Context) (*pb.ModifyProcessInstanceResponse, error)
}

type ModifyInstanceCommand struct {
	Command
	request pb.ModifyProcessInstanceRequest
}

func (cmd *ModifyInstanceCommand) ProcessInstanceKey(key int64) ModifyInstanceCommandStep2 {
	cmd.request.ProcessInstanceKey = key
	return cmd
}

func (cmd *ModifyInstanceCommand) ActivateElement(elementID string) ModifyInstanceActivateStep {
	return cmd.ActivateElementWithAncestor(elementID, 0)
}

func (cmd *ModifyInstanceCommand) ActivateElementWithAncestor(elementID string, ancestorElementInstanceKey int64) ModifyInstanceActivateStep {
	cmd.request.ActivateInstructions = append(cmd.request.ActivateInstructions, &pb.ModifyProcessInstanceRequest_ActivateInstruction{
		ElementId:                  elementID,
		AncestorElementInstanceKey: ancestorElementInstanceKey,
	})
	return cmd
}

func (cmd *ModifyInstanceCommand) TerminateElement(elementInstanceKey int64) DispatchModifyInstanceCommand {
	cmd.request.TerminateInstructions = append(cmd.request.TerminateInstructions, &pb.ModifyProcessInstanceRequest_TerminateInstruction{
		ElementInstanceKey: elementInstanceKey,
	})
	return cmd
}

func (cmd *ModifyInstanceCommand) VariablesFromString(variables string) (ModifyInstanceActivateStep, error) {
	return cmd.VariablesFromStringInScope("", variables)
}

func (cmd *ModifyInstanceCommand) VariablesFromMap(variables map[string]interface{}) (ModifyInstanceActivateStep, error) {
	return cmd.VariablesFromObjectInScope("", variables)
}

func (cmd *ModifyInstanceCommand) VariablesFromObject(variables interface{}) (ModifyInstanceActivateStep, error) {
	return cmd.VariablesFromObjectInScope("", variables)
}

func (cmd *ModifyInstanceCommand) VariablesFromStringInScope(scopeID string, variables string) (ModifyInstanceActivateStep, error) {
	if err := cmd.mixin.Validate("variables", variables); err != nil {
		return nil, err
	}
	return cmd.addVariables(scopeID, variables), nil
}

func (cmd *ModifyInstanceCommand) VariablesFromMapInScope(scopeID string, variables map[string]interface{}) (ModifyInstanceActivateStep, error) {
	return cmd.VariablesFromObjectInScope(scopeID, variables)
}

func (cmd *ModifyInstanceCommand) VariablesFromObjectInScope(scopeID string, variables interface{}) (ModifyInstanceActivateStep, error) {
	value, err := cmd.mixin.AsJSON("variables", variables, false)
	if err != nil {
		return nil, err
	}
	return cmd.addVariables(scopeID, value), nil
}

// addVariables attaches variables to the last activate instruction. The step types only
// expose the variable methods right after an activation, so there always is one.
func (cmd *ModifyInstanceCommand) addVariables(scopeID string, variables string) ModifyInstanceActivateStep {
	last := cmd.request.ActivateInstructions[len(cmd.request.ActivateInstructions)-1]
	last.VariableInstructions = append(last.VariableInstructions, &pb.ModifyProcessInstanceRequest_VariableInstruction{
		Variables: variables,
		ScopeId:   scopeID,
	})
	return cmd
}

func (cmd *ModifyInstanceCommand) Send(ctx context.Context) (*pb.ModifyProcessInstanceResponse, error) {
	response, err := cmd.gateway.ModifyProcessInstance(ctx, &cmd.request)
	if cmd.shouldRetry(ctx, err) {
		return cmd.Send(ctx)
	}

	return response, err
}

func NewModifyInstanceCommand(gateway pb.GatewayClient, pred retryPredicate) ModifyInstanceCommandStep1 {
	return &ModifyInstanceCommand{
		Command: Command{
			mixin:       utils.NewJSONStringSerializer(),
			gateway:     gateway,
			shouldRetry: pred,
		},
	}
}
