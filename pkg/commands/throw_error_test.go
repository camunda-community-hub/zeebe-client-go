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

package commands

import (
	"context"
	"testing"

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/mock_pb"
	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
	"github.com/golang/mock/gomock"
)

func TestThrowErrorCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: "someErrorMessage",
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	response, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage("someErrorMessage").Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromString(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{\"foo\":\"bar\"}"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromString(variables)
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromStringer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{\"foo\":\"bar\"}"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromStringer(DataType{Foo: "bar"})
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromObject(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{\"foo\":\"bar\"}"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromObject(DataType{Foo: "bar"})
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromObjectOmitempty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{}"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromObject(DataType{Foo: ""})
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromObjectIgnoreOmitempty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{\"foo\":\"\"}"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromObjectIgnoreOmitempty(DataType{Foo: ""})
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}

func TestThrowErrorCommand_VariablesFromMap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_pb.NewMockGatewayClient(ctrl)

	errorMessage := "something went wrong"
	variables := "{\"foo\":\"bar\"}"
	variableMaps := make(map[string]interface{})
	variableMaps["foo"] = "bar"

	request := &pb.ThrowErrorRequest{
		JobKey:       123,
		ErrorCode:    "someErrorCode",
		ErrorMessage: errorMessage,
		Variables:    variables,
	}
	stub := &pb.ThrowErrorResponse{}

	client.EXPECT().ThrowError(gomock.Any(), &utils.RPCTestMsg{Msg: request}).Return(stub, nil)

	command := NewThrowErrorCommand(client, func(context.Context, error) bool { return false })

	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	variablesCommand, err := command.JobKey(123).ErrorCode("someErrorCode").ErrorMessage(errorMessage).VariablesFromMap(variableMaps)
	if err != nil {
		t.Error("Failed to set variables: ", err)
	}

	response, err := variablesCommand.Send(ctx)

	if err != nil {
		t.Errorf("Failed to send request")
	}

	if response != stub {
		t.Errorf("Failed to receive response")
	}
}
