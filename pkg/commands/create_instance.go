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
	"fmt"
	"github.com/zeebe-io/zeebe/clients/go/internal/utils"
	"github.com/zeebe-io/zeebe/clients/go/pkg/pb"
)

const LatestVersion = -1

type DispatchCreateInstanceCommand interface {
	Send(context.Context) (*pb.CreateWorkflowInstanceResponse, error)
}

type DispatchCreateInstanceWithResultCommand interface {
	Send(context.Context) (*pb.CreateWorkflowInstanceWithResultResponse, error)
}

type CreateInstanceCommandStep1 interface {
	BPMNProcessId(string) CreateInstanceCommandStep2
	WorkflowKey(int64) CreateInstanceCommandStep3
}

type CreateInstanceCommandStep2 interface {
	Version(int32) CreateInstanceCommandStep3
	LatestVersion() CreateInstanceCommandStep3
}

type CreateInstanceCommandStep3 interface {
	DispatchCreateInstanceCommand

	// Expected to be valid JSON string
	VariablesFromString(string) (CreateInstanceCommandStep3, error)

	// Expected to construct a valid JSON string
	VariablesFromStringer(fmt.Stringer) (CreateInstanceCommandStep3, error)

	// Expected that object is JSON serializable
	VariablesFromObject(interface{}) (CreateInstanceCommandStep3, error)
	VariablesFromObjectIgnoreOmitempty(interface{}) (CreateInstanceCommandStep3, error)
	VariablesFromMap(map[string]interface{}) (CreateInstanceCommandStep3, error)

	WithResult() CreateInstanceWithResultCommandStep1
}

type CreateInstanceWithResultCommandStep1 interface {
	DispatchCreateInstanceWithResultCommand

	FetchVariables(variableNames ...string) CreateInstanceWithResultCommandStep1
}

type CreateInstanceCommand struct {
	Command
	request pb.CreateWorkflowInstanceRequest
}

type CreateInstanceWithResultCommand struct {
	Command
	request pb.CreateWorkflowInstanceWithResultRequest
}

func (cmd *CreateInstanceCommand) VariablesFromString(variables string) (CreateInstanceCommandStep3, error) {
	err := cmd.mixin.Validate("variables", variables)
	if err != nil {
		return nil, err
	}

	cmd.request.Variables = variables
	return cmd, nil
}

func (cmd *CreateInstanceCommand) VariablesFromStringer(variables fmt.Stringer) (CreateInstanceCommandStep3, error) {
	return cmd.VariablesFromString(variables.String())
}

func (cmd *CreateInstanceCommand) VariablesFromObject(variables interface{}) (CreateInstanceCommandStep3, error) {
	value, err := cmd.mixin.AsJson("variables", variables, false)
	if err != nil {
		return nil, err
	}

	cmd.request.Variables = value
	return cmd, err
}

func (cmd *CreateInstanceCommand) VariablesFromObjectIgnoreOmitempty(variables interface{}) (CreateInstanceCommandStep3, error) {
	value, err := cmd.mixin.AsJson("variables", variables, true)
	if err != nil {
		return nil, err
	}

	cmd.request.Variables = value
	return cmd, err
}

func (cmd *CreateInstanceCommand) VariablesFromMap(variables map[string]interface{}) (CreateInstanceCommandStep3, error) {
	return cmd.VariablesFromObject(variables)
}

func (cmd *CreateInstanceCommand) Version(version int32) CreateInstanceCommandStep3 {
	cmd.request.Version = version
	return cmd
}

func (cmd *CreateInstanceCommand) LatestVersion() CreateInstanceCommandStep3 {
	cmd.request.Version = LatestVersion
	return cmd
}

func (cmd *CreateInstanceCommand) WorkflowKey(key int64) CreateInstanceCommandStep3 {
	cmd.request.WorkflowKey = key
	return cmd
}

func (cmd *CreateInstanceCommand) BPMNProcessId(id string) CreateInstanceCommandStep2 {
	cmd.request.BpmnProcessId = id
	return cmd
}

func (cmd *CreateInstanceCommand) WithResult() CreateInstanceWithResultCommandStep1 {
	return &CreateInstanceWithResultCommand{
		request: pb.CreateWorkflowInstanceWithResultRequest{
			Request: &cmd.request,
		},
		Command: Command{
			mixin:       cmd.mixin,
			gateway:     cmd.gateway,
			shouldRetry: cmd.shouldRetry,
		},
	}
}

func (cmd *CreateInstanceWithResultCommand) FetchVariables(variableNames ...string) CreateInstanceWithResultCommandStep1 {
	cmd.request.FetchVariables = variableNames
	return cmd
}

func (cmd *CreateInstanceCommand) Send(ctx context.Context) (*pb.CreateWorkflowInstanceResponse, error) {
	response, err := cmd.gateway.CreateWorkflowInstance(ctx, &cmd.request)
	if cmd.shouldRetry(ctx, err) {
		return cmd.Send(ctx)
	}

	return response, err
}

func (cmd *CreateInstanceWithResultCommand) Send(ctx context.Context) (*pb.CreateWorkflowInstanceWithResultResponse, error) {
	cmd.request.RequestTimeout = getLongPollingMillis(ctx)

	response, err := cmd.gateway.CreateWorkflowInstanceWithResult(ctx, &cmd.request)
	if cmd.shouldRetry(ctx, err) {
		return cmd.Send(ctx)
	}

	return response, err
}

func NewCreateInstanceCommand(gateway pb.GatewayClient, pred retryPredicate) CreateInstanceCommandStep1 {
	return &CreateInstanceCommand{
		Command: Command{
			mixin:       utils.NewJsonStringSerializer(),
			gateway:     gateway,
			shouldRetry: pred,
		},
	}
}
