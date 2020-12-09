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

package zbc

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/zeebe-io/zeebe/clients/go/internal/embedded"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"google.golang.org/grpc"

	"github.com/zeebe-io/zeebe/clients/go/pkg/commands"
	"github.com/zeebe-io/zeebe/clients/go/pkg/pb"
	"github.com/zeebe-io/zeebe/clients/go/pkg/worker"
)

const DefaultRequestTimeout = 15 * time.Second
const DefaultKeepAlive = 45 * time.Second
const InsecureEnvVar = "ZEEBE_INSECURE_CONNECTION"
const CaCertificatePath = "ZEEBE_CA_CERTIFICATE_PATH"
const KeepAliveEnvVar = "ZEEBE_KEEP_ALIVE"
const GatewayAddressEnvVar = "ZEEBE_ADDRESS"

type ClientImpl struct {
	gateway             pb.GatewayClient
	connection          *grpc.ClientConn
	credentialsProvider CredentialsProvider
}

type ClientConfig struct {
	GatewayAddress         string
	UsePlaintextConnection bool
	CaCertificatePath      string
	CredentialsProvider    CredentialsProvider

	// KeepAlive can be used configure how often keep alive messages should be sent to the gateway. These will be sent
	// whether or not there are active requests. Negative values will result in error and zero will result in the default
	// of 45 seconds being used
	KeepAlive time.Duration

	DialOpts []grpc.DialOption
}

// ErrFileNotFound is returned whenever a file can't be found at the provided path. Use this value to do error comparison.
const ErrFileNotFound = Error("file not found")

type Error string

func (e Error) Error() string {
	return string(e)
}

func (c *ClientImpl) NewTopologyCommand() *commands.TopologyCommand {
	return commands.NewTopologyCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewDeployWorkflowCommand() *commands.DeployCommand {
	return commands.NewDeployCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewPublishMessageCommand() commands.PublishMessageCommandStep1 {
	return commands.NewPublishMessageCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewResolveIncidentCommand() commands.ResolveIncidentCommandStep1 {
	return commands.NewResolveIncidentCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewCreateInstanceCommand() commands.CreateInstanceCommandStep1 {
	return commands.NewCreateInstanceCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewCancelInstanceCommand() commands.CancelInstanceStep1 {
	return commands.NewCancelInstanceCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewCompleteJobCommand() commands.CompleteJobCommandStep1 {
	return commands.NewCompleteJobCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewFailJobCommand() commands.FailJobCommandStep1 {
	return commands.NewFailJobCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewUpdateJobRetriesCommand() commands.UpdateJobRetriesCommandStep1 {
	return commands.NewUpdateJobRetriesCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewSetVariablesCommand() commands.SetVariablesCommandStep1 {
	return commands.NewSetVariablesCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewActivateJobsCommand() commands.ActivateJobsCommandStep1 {
	return commands.NewActivateJobsCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewThrowErrorCommand() commands.ThrowErrorCommandStep1 {
	return commands.NewThrowErrorCommand(c.gateway, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) NewJobWorker() worker.JobWorkerBuilderStep1 {
	return worker.NewJobWorkerBuilder(c.gateway, c, c.credentialsProvider.ShouldRetryRequest)
}

func (c *ClientImpl) Close() error {
	return c.connection.Close()
}

func NewClient(config *ClientConfig) (Client, error) {
	err := applyClientEnvOverrides(config)
	if err != nil {
		return nil, err
	}

	err = configureConnectionSecurity(config)
	if err != nil {
		return nil, err
	}

	err = configureCredentialsProvider(config)
	if err != nil {
		return nil, err
	}

	err = configureKeepAlive(config)
	if err != nil {
		return nil, err
	}

	config.DialOpts = append(config.DialOpts, grpc.WithUserAgent("zeebe-client-go/"+getVersion()))

	conn, err := grpc.Dial(config.GatewayAddress, config.DialOpts...)
	if err != nil {
		return nil, err
	}

	return &ClientImpl{
		gateway:             pb.NewGatewayClient(conn),
		connection:          conn,
		credentialsProvider: config.CredentialsProvider,
	}, nil
}

func applyClientEnvOverrides(config *ClientConfig) error {
	if insecureConn := env.get(InsecureEnvVar); insecureConn != "" {
		config.UsePlaintextConnection = insecureConn == "true"
	}

	if caCertificatePath := env.get(CaCertificatePath); caCertificatePath != "" {
		config.CaCertificatePath = caCertificatePath
	}

	if gatewayAddress := env.get(GatewayAddressEnvVar); gatewayAddress != "" {
		config.GatewayAddress = gatewayAddress
	}

	if val := env.get(KeepAliveEnvVar); val != "" {
		keepAlive, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("keep alive must be expressed as positive number of milliseconds: %w", err)
		}

		config.KeepAlive = time.Duration(keepAlive) * time.Millisecond
	}

	return nil
}

func configureCredentialsProvider(config *ClientConfig) error {
	if config.CredentialsProvider == nil && shouldUseDefaultCredentialsProvider() {
		if err := setDefaultCredentialsProvider(config); err != nil {
			return err
		}
	}

	if config.CredentialsProvider != nil {
		if config.UsePlaintextConnection {
			log.Println("Warning: The configured security level does not guarantee that the credentials will be confidential. If this unintentional, please enable transport security.")
		}

		callCredentials := &callCredentials{credentialsProvider: config.CredentialsProvider}
		config.DialOpts = append(config.DialOpts, grpc.WithPerRPCCredentials(callCredentials))
	} else {
		config.CredentialsProvider = &noopCredentialsProvider{}
	}

	return nil
}

func shouldUseDefaultCredentialsProvider() bool {
	return env.get(OAuthClientSecretEnvVar) != "" || env.get(OAuthClientIdEnvVar) != ""
}

func setDefaultCredentialsProvider(config *ClientConfig) error {
	var audience string
	index := strings.LastIndex(config.GatewayAddress, ":")
	if index > 0 {
		audience = config.GatewayAddress[0:index]
	}

	provider, err := NewOAuthCredentialsProvider(&OAuthProviderConfig{Audience: audience})
	if err != nil {
		return err
	}

	config.CredentialsProvider = provider
	return nil
}

func configureConnectionSecurity(config *ClientConfig) error {
	if !config.UsePlaintextConnection {
		var creds credentials.TransportCredentials

		if config.CaCertificatePath == "" {
			creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
		} else if _, err := os.Stat(config.CaCertificatePath); os.IsNotExist(err) {
			return fmt.Errorf("expected to find CA certificate but no such file at '%s': %w", config.CaCertificatePath, ErrFileNotFound)
		} else {
			creds, err = credentials.NewClientTLSFromFile(config.CaCertificatePath, "")
			if err != nil {
				return err
			}
		}

		config.DialOpts = append(config.DialOpts, grpc.WithTransportCredentials(creds))
	} else {
		config.DialOpts = append(config.DialOpts, grpc.WithInsecure())
	}

	return nil
}

func configureKeepAlive(config *ClientConfig) error {
	keepAlive := DefaultKeepAlive

	if config.KeepAlive < time.Duration(0) {
		return errors.New("keep alive must be a positive duration")
	} else if config.KeepAlive != time.Duration(0) {
		keepAlive = config.KeepAlive
	}
	config.DialOpts = append(config.DialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: keepAlive}))

	return nil
}

func getVersion() string {
	zbVersion := "development"
	if readVersion, err := embedded.Asset("VERSION"); err == nil {
		zbVersion = strings.TrimSpace(string(readVersion))
	}

	return zbVersion
}
