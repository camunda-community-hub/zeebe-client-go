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
package test

import (
	"context"
	"github.com/camunda-cloud/zeebe/clients/go/internal/containersuite"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

type integrationTestSuite struct {
	*containersuite.ContainerSuite
	client zbc.Client
}

func TestIntegration(t *testing.T) {
	suite.Run(t, &integrationTestSuite{
		ContainerSuite: &containersuite.ContainerSuite{
			WaitTime:       time.Second,
			ContainerImage: "camunda/zeebe:current-test",
		},
	})
}

func (s *integrationTestSuite) SetupSuite() {
	var err error
	s.ContainerSuite.SetupSuite()

	s.client, err = zbc.NewClient(&zbc.ClientConfig{
		GatewayAddress:         s.GatewayAddress,
		UsePlaintextConnection: true,
	})
	if err != nil {
		s.T().Fatal(err)
	}
}

func (s *integrationTestSuite) TearDownSuite() {
	err := s.client.Close()
	if err != nil {
		s.T().Fatal(err)
	}

	s.ContainerSuite.TearDownSuite()
}
func (s *integrationTestSuite) TestTopology() {
	// when
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := s.client.NewTopologyCommand().Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// then
	s.EqualValues(1, response.GetClusterSize())
	s.EqualValues(1, response.GetPartitionsCount())
	s.EqualValues(1, response.GetReplicationFactor())
	s.NotEmpty(response.GetGatewayVersion())

	for _, broker := range response.GetBrokers() {
		s.EqualValues(response.GetGatewayVersion(), broker.GetVersion())
	}
}

func (s *integrationTestSuite) TestDeployProcess() {
	// when
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deployment, err := s.client.NewDeployProcessCommand().AddResourceFile("testdata/service_task.bpmn").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// then
	s.Greater(deployment.GetKey(), int64(0))

	process := deployment.GetProcesses()[0]
	s.NotNil(process)
	s.EqualValues("deploy_process", process.GetBpmnProcessId())
	s.EqualValues(int32(1), process.GetVersion())
	s.Greater(process.GetProcessDefinitionKey(), int64(0))
}

func (s *integrationTestSuite) TestCreateInstance() {
	// given
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deployment, err := s.client.NewDeployProcessCommand().AddResourceFile("testdata/service_task.bpmn").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// when
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	process := deployment.GetProcesses()[0]
	processInstance, err := s.client.NewCreateInstanceCommand().BPMNProcessId("deploy_process").Version(process.GetVersion()).Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// then
	s.EqualValues(process.GetVersion(), processInstance.GetVersion())
	s.EqualValues(process.GetProcessDefinitionKey(), processInstance.GetProcessDefinitionKey())
	s.EqualValues(process.GetBpmnProcessId(), processInstance.GetBpmnProcessId())
	s.Greater(processInstance.GetProcessInstanceKey(), int64(0))
}

func (s *integrationTestSuite) TestActivateJobs() {
	// given
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deployment, err := s.client.NewDeployProcessCommand().AddResourceFile("testdata/service_task.bpmn").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	process := deployment.GetProcesses()[0]
	_, err = s.client.NewCreateInstanceCommand().ProcessDefinitionKey(process.GetProcessDefinitionKey()).Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// when
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs, err := s.client.NewActivateJobsCommand().JobType("task").MaxJobsToActivate(1).Timeout(time.Minute * 5).WorkerName("worker").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// then
	for _, job := range jobs {
		s.EqualValues(process.GetProcessDefinitionKey(), job.GetProcessDefinitionKey())
		s.EqualValues(process.GetBpmnProcessId(), job.GetBpmnProcessId())
		s.EqualValues("service_task", job.GetElementId())
		s.Greater(job.GetRetries(), int32(0))

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		jobResponse, err := s.client.NewCompleteJobCommand().JobKey(job.Key).Send(ctx)
		if err != nil {
			s.T().Fatal(err)
		}

		if jobResponse == nil {
			s.T().Fatal("Empty complete job response")
		}
	}
}

func (s *integrationTestSuite) TestFailJob() {
	// given
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deployment, err := s.client.NewDeployProcessCommand().AddResourceFile("testdata/service_task.bpmn").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	process := deployment.GetProcesses()[0]
	_, err = s.client.NewCreateInstanceCommand().ProcessDefinitionKey(process.GetProcessDefinitionKey()).Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// when
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs, err := s.client.NewActivateJobsCommand().JobType("task").MaxJobsToActivate(1).Timeout(time.Minute * 5).WorkerName("worker").Send(ctx)
	if err != nil {
		s.T().Fatal(err)
	}

	// then
	for _, job := range jobs {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		failedJob, err := s.client.NewFailJobCommand().JobKey(job.GetKey()).Retries(0).Send(ctx)
		if err != nil {
			s.T().Fatal(err)
		}

		if failedJob == nil {
			s.T().Fatal("Empty fail job response")
		}
	}
}
