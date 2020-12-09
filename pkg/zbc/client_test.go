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
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/zeebe-io/zeebe/clients/go/pkg/pb"
)

type clientTestSuite struct {
	*envSuite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, &clientTestSuite{envSuite: new(envSuite)})
}

func (s *clientTestSuite) TestClientWithTls() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, err := NewClient(&ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/chain.cert.pem",
	})

	s.NoError(err)

	// when
	_, err = client.NewTopologyCommand().Send(context.Background())

	// then
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.EqualValues(codes.Unimplemented, grpcStatus.Code())
	}
}

func (s *clientTestSuite) TestInsecureEnvVar() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()
	parts := strings.Split(lis.Addr().String(), ":")

	// when
	config := &ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/chain.cert.pem",
	}
	env.set(InsecureEnvVar, "true")

	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues(true, config.UsePlaintextConnection)
}

func (s *clientTestSuite) TestGatewayAddressEnvVar() {
	// given
	lis, grpcServer := createServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()
	parts := strings.Split(lis.Addr().String(), ":")

	// when
	config := &ClientConfig{
		UsePlaintextConnection: true,
		GatewayAddress:         "wrong_address",
	}
	env.set(GatewayAddressEnvVar, fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]))

	cli, err := NewClient(config)
	s.NoError(err)

	_, err = cli.NewTopologyCommand().Send(context.Background())

	// then
	if errStat, ok := status.FromError(err); ok {
		s.EqualValues(codes.Unimplemented, errStat.Code())
	}
	s.EqualValues(fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]), config.GatewayAddress)
}

func (s *clientTestSuite) TestCaCertificateEnvVar() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()
	parts := strings.Split(lis.Addr().String(), ":")

	// when
	config := &ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/wrong.cert",
	}
	env.set(CaCertificatePath, "testdata/chain.cert.pem")

	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues("testdata/chain.cert.pem", config.CaCertificatePath)
}

func (s *clientTestSuite) TestClientWithoutTls() {
	// given
	lis, grpcServer := createServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, err := NewClient(&ClientConfig{
		GatewayAddress:         fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		UsePlaintextConnection: true,
		CaCertificatePath:      "testdata/chain.cert.pem",
	})

	s.NoError(err)

	// when
	_, err = client.NewTopologyCommand().Send(context.Background())

	// then
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.Equal(codes.Unimplemented, grpcStatus.Code())
	}
}

func (s *clientTestSuite) TestClientWithDefaultRootCa() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, err := NewClient(&ClientConfig{
		GatewayAddress: fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
	})

	s.NoError(err)

	// then
	_, err = client.NewTopologyCommand().Send(context.Background())

	// when
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		// asserts that an attempt was made to validate the certificate (which fails because it's not installed)
		s.Contains(grpcStatus.Message(), "certificate signed by unknown authority")
	}
}

func (s *clientTestSuite) TestClientWithPathToNonExistingFile() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	wrongPath := "non.existing"

	// when
	_, err := NewClient(&ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: wrongPath,
	})

	// then
	s.Error(err)
	s.True(errors.Is(err, ErrFileNotFound), "expected error to be of type 'FileNotFound'")
}

func (s *clientTestSuite) TestClientWithDefaultCredentialsProvider() {
	// given
	lis, grpcServer := createServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	authzServer := mockAuthorizationServerWithAudience(s.T(), &mutableToken{value: accessToken}, "0.0.0.0")
	defer authzServer.Close()

	env.set(OAuthClientSecretEnvVar, clientSecret)
	env.set(OAuthClientIdEnvVar, clientID)
	env.set(OAuthAuthorizationUrlEnvVar, authzServer.URL)

	parts := strings.Split(lis.Addr().String(), ":")
	config := &ClientConfig{
		GatewayAddress:         fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		UsePlaintextConnection: true,
	}
	client, err := NewClient(config)
	s.NoError(err)

	// when
	_, err = client.NewTopologyCommand().Send(context.Background())

	// then
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.Equal(codes.Unimplemented, grpcStatus.Code())
	}
}

func (s *clientTestSuite) TestKeepAlive() {
	// given
	keepAlive := 2 * time.Minute
	config := &ClientConfig{
		GatewayAddress:         "0.0.0.0:0",
		UsePlaintextConnection: true,
		KeepAlive:              keepAlive,
	}

	// when
	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.Equal(keepAlive, config.KeepAlive)

}

func (s *clientTestSuite) TestOverrideKeepAliveWithEnvVar() {
	// given
	keepAlive := 2 * 60 * 1000

	env.set(KeepAliveEnvVar, strconv.Itoa(keepAlive))
	config := &ClientConfig{
		GatewayAddress:         "0.0.0.0:0",
		UsePlaintextConnection: true,
		KeepAlive:              5 * time.Second,
	}

	// when
	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues(keepAlive, config.KeepAlive.Milliseconds())
}

func (s *clientTestSuite) TestRejectNegativeDuration() {
	// given
	config := &ClientConfig{
		GatewayAddress:         "0.0.0.0:0",
		UsePlaintextConnection: true,
		KeepAlive:              -5 * time.Second,
	}

	// when
	_, err := NewClient(config)

	// then
	s.Error(err)
}

func (s *clientTestSuite) TestRejectNegativeDurationAsEnvVar() {
	// given
	env.set(KeepAliveEnvVar, "-100")
	config := &ClientConfig{
		GatewayAddress:         "0.0.0.0:0",
		UsePlaintextConnection: true,
	}

	// when
	_, err := NewClient(config)

	// then
	s.Error(err)
}

func (s *clientTestSuite) TestCommandExpireWithContext() {
	// given
	blockReq := make(chan struct{})
	defer close(blockReq)
	lis, server := createServerWithUnaryInterceptor(func(_ context.Context, _ interface{}, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (interface{}, error) {
		<-blockReq
		return nil, nil
	})
	go server.Serve(lis)
	defer server.Stop()

	client, err := NewClient(&ClientConfig{
		GatewayAddress:         lis.Addr().String(),
		UsePlaintextConnection: true,
	})
	s.NoError(err)

	// when
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	cmdFinished := make(chan struct{})
	go func() {
		_, err = client.NewTopologyCommand().Send(ctx)
		close(cmdFinished)
	}()

	// then
	select {
	case <-cmdFinished:
	case <-time.After(2 * time.Second):
		s.FailNow("expected command to fail with deadline exceeded, but blocked instead")
	}

	code := status.Code(err)
	if code != codes.DeadlineExceeded {
		s.FailNow(fmt.Sprintf("expected command to fail with deadline exceeded, but got %s instead", code.String()))
	}
}

func (s *clientTestSuite) TestClientWithEmptyDialOptions() {
	// given
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, err := NewClient(&ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/chain.cert.pem",
		DialOpts:          make([]grpc.DialOption, 0),
	})

	s.NoError(err)

	// when
	_, err = client.NewTopologyCommand().Send(context.Background())

	// then
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.EqualValues(codes.Unimplemented, grpcStatus.Code())
	}
}

func createSecureServer() (net.Listener, *grpc.Server) {
	creds, _ := credentials.NewServerTLSFromFile("testdata/chain.cert.pem", "testdata/private.key.pem")
	return createServer(grpc.Creds(creds))
}

func createServer(opts ...grpc.ServerOption) (net.Listener, *grpc.Server) {
	lis, _ := net.Listen("tcp", "0.0.0.0:0")
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterGatewayServer(grpcServer, &pb.UnimplementedGatewayServer{})
	return lis, grpcServer
}
