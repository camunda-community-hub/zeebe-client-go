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
	"github.com/camunda-cloud/zeebe/clients/go/pkg/pb"
	"io/ioutil"
	"log"
)

type DeployCommand struct {
	Command
	request pb.DeployProcessRequest
}

func (cmd *DeployCommand) AddResourceFile(path string) *DeployCommand {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	return cmd.AddResource(b, path, pb.ProcessRequestObject_FILE)
}

func (cmd *DeployCommand) AddResource(definition []byte, name string, resourceType pb.ProcessRequestObject_ResourceType) *DeployCommand {
	cmd.request.Processes = append(cmd.request.Processes, &pb.ProcessRequestObject{Definition: definition, Name: name, Type: resourceType})
	return cmd
}

func (cmd *DeployCommand) Send(ctx context.Context) (*pb.DeployProcessResponse, error) {
	response, err := cmd.gateway.DeployProcess(ctx, &cmd.request)
	if cmd.shouldRetry(ctx, err) {
		return cmd.Send(ctx)
	}

	return response, err
}

func NewDeployCommand(gateway pb.GatewayClient, pred retryPredicate) *DeployCommand {
	return &DeployCommand{
		Command: Command{
			gateway:     gateway,
			shouldRetry: pred,
		},
	}
}
