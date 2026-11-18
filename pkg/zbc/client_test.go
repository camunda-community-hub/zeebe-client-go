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

	"github.com/camunda-community-hub/zeebe-client-go/v8/internal/utils"
	"google.golang.org/grpc/metadata"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/camunda-community-hub/zeebe-client-go/v8/pkg/pb"
)

type clientTestSuite struct {
	*envSuite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, &clientTestSuite{envSuite: new(envSuite)})
}

func (s *clientTestSuite) TestClientWithTls() {
	// given
	lis, grpcServer := createSecureServer(false)

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

func (s *clientTestSuite) TestClientWithOverrideAuthority() {
	// given
	lis, grpcServer := createSecureServer(true)
	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	gatewayAddress := fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1])
	validClient, err := NewClient(&ClientConfig{
		GatewayAddress:    gatewayAddress,
		CaCertificatePath: "testdata/chain.cert.san.pem",
		OverrideAuthority: "gateway.net",
	})
	s.NoError(err)

	invalidClient, err := NewClient(&ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/chain.cert.san.pem",
		OverrideAuthority: "wrong-host",
	})
	s.NoError(err)

	// when - then with valid client, connection should succeed but the call is unimplemented
	_, err = validClient.NewTopologyCommand().Send(context.Background())
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.EqualValues(codes.Unimplemented, grpcStatus.Code())
	}

	// when - then with invalid client, connection should fail and the server appear unavailable
	_, err = invalidClient.NewTopologyCommand().Send(context.Background())
	s.Error(err)
	if grpcStatus, ok := status.FromError(err); ok {
		s.EqualValues(codes.Unavailable, grpcStatus.Code())
	}
}

func (s *clientTestSuite) TestInsecureEnvVar() {
	// given
	lis, grpcServer := createSecureServer(false)

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
	lis, grpcServer := createServerWithDefaultAddress()

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

func (s *clientTestSuite) TestDefaultUserAgent() {
	// given
	var incomingContext = make(map[string][]string)
	lis, server := createServerWithUnaryInterceptor(func(ctx context.Context, _ interface{}, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (interface{}, error) {
		incomingContext, _ = metadata.FromIncomingContext(ctx)
		return nil, nil
	})
	go server.Serve(lis)
	defer server.Stop()

	// when
	client, err := NewClient(&ClientConfig{
		GatewayAddress:         lis.Addr().String(),
		UsePlaintextConnection: true,
	})
	s.Require().NoError(err)
	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	_, _ = client.NewTopologyCommand().Send(ctx)
	userAgent := incomingContext["user-agent"]

	// then
	s.Require().Len(userAgent, 1)
	s.Require().Contains(userAgent[0], "zeebe-client-go/"+Version)
}

func (s *clientTestSuite) TestSpecificUserAgent() {
	// given
	var incomingContext = make(map[string][]string)
	lis, server := createServerWithUnaryInterceptor(func(ctx context.Context, _ interface{}, _ *grpc.UnaryServerInfo, _ grpc.UnaryHandler) (interface{}, error) {
		incomingContext, _ = metadata.FromIncomingContext(ctx)
		return nil, nil
	})
	go server.Serve(lis)
	defer server.Stop()

	// when
	client, err := NewClient(&ClientConfig{
		GatewayAddress:         lis.Addr().String(),
		UsePlaintextConnection: true,
		UserAgent:              "anotherUserAgentLikeZbctl",
	})
	s.Require().NoError(err)
	ctx, cancel := context.WithTimeout(context.Background(), utils.DefaultTestTimeout)
	defer cancel()

	_, _ = client.NewTopologyCommand().Send(ctx)
	userAgent := incomingContext["user-agent"]

	// then
	s.Require().Len(userAgent, 1)
	s.Require().Contains(userAgent[0], "anotherUserAgentLikeZbctl")
}

func (s *clientTestSuite) TestCaCertificateEnvVar() {
	// given
	lis, grpcServer := createSecureServer(false)

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

func (s *clientTestSuite) TestOverrideAuthorityEnvVar() {
	// given
	lis, grpcServer := createSecureServer(true)

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()
	parts := strings.Split(lis.Addr().String(), ":")

	// when
	config := &ClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "testdata/chain.cert.san.pem",
		OverrideAuthority: "wrong-authority",
	}
	env.set(OverrideAuthorityEnvVar, "gateway.net")

	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues("gateway.net", config.OverrideAuthority)
}

func (s *clientTestSuite) TestClientWithoutTls() {
	// given
	lis, grpcServer := createServerWithDefaultAddress()

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
	lis, grpcServer := createSecureServer(false)

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
		// asserts that an attempt was made to validate the certificate (which fails because it's not installed).
		// The reason differs by platform verifier: "certificate signed by unknown authority" on Linux,
		// "certificate is not standards compliant" on macOS.
		s.Contains(grpcStatus.Message(), "tls: failed to verify certificate")
	}
}

func (s *clientTestSuite) TestClientWithPathToNonExistingFile() {
	// given
	lis, grpcServer := createSecureServer(false)

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
	lis, grpcServer := createServerWithDefaultAddress()

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
	lis, grpcServer := createSecureServer(false)

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

func (s *clientTestSuite) TestOverrideHostAndPortEnvVar() {
	// given
	address := "127.1"
	port := "9090"

	env.set(GatewayHostEnvVar, address)
	env.set(GatewayPortEnvVar, port)
	config := &ClientConfig{
		GatewayAddress:         "wrong_address",
		UsePlaintextConnection: true,
	}

	// when
	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues(fmt.Sprintf("%s:%s", address, port), config.GatewayAddress)
}

func (s *clientTestSuite) TestOverrideIPv6HostEnvVar() {
	// given
	env.set(GatewayHostEnvVar, "::1")
	env.set(GatewayPortEnvVar, "9090")
	config := &ClientConfig{
		GatewayAddress:         "wrong_address",
		UsePlaintextConnection: true,
	}

	// when
	_, err := NewClient(config)

	// then
	s.NoError(err)
	s.EqualValues("[::1]:9090", config.GatewayAddress)
}

func (s *clientTestSuite) TestOverrideIPv6HostEnvVarVariants() {
	for _, tc := range []struct{ host, port, want string }{
		{"[::1]", "9090", "[::1]:9090"},
		{"::1", "", "[::1]:" + DefaultAddressPort},
		{"[::1]", "", "[::1]:" + DefaultAddressPort},
	} {
		env.set(GatewayHostEnvVar, tc.host)
		if tc.port != "" {
			env.set(GatewayPortEnvVar, tc.port)
		} else {
			env.unset(GatewayPortEnvVar)
		}
		config := &ClientConfig{GatewayAddress: "wrong_address", UsePlaintextConnection: true}
		s.NoError(applyClientEnvOverrides(config))
		s.EqualValues(tc.want, config.GatewayAddress, "host=%q port=%q", tc.host, tc.port)
	}
}

func (s *clientTestSuite) TestGatewayAddressWithoutPortDefaultsToGatewayPortWhenPlaintext() {
	for _, tc := range []struct {
		name      string
		address   string
		envVar    bool
		plaintext bool
		want      string
	}{
		{name: "plaintext host", address: "127.0.0.1", plaintext: true, want: "127.0.0.1:26500"},
		{name: "plaintext host from ZEEBE_ADDRESS", address: "127.0.0.1", envVar: true, plaintext: true, want: "127.0.0.1:26500"},
		{name: "plaintext hostname", address: "zeebe", plaintext: true, want: "zeebe:26500"},
		{name: "plaintext bracketed IPv6", address: "[::1]", plaintext: true, want: "[::1]:26500"},
		{name: "plaintext with port", address: "zeebe:1234", plaintext: true, want: "zeebe:1234"},
		{name: "plaintext gRPC target URI", address: "dns:///zeebe", plaintext: true, want: "dns:///zeebe"},
		{name: "plaintext bare IPv6 is ambiguous", address: "::1", plaintext: true, want: "::1"},
		// TLS keeps gRPC's default (443), which Camunda SaaS addresses rely on.
		{name: "TLS host", address: "abc.bru-2.zeebe.camunda.io", want: "abc.bru-2.zeebe.camunda.io"},
		{name: "TLS host from ZEEBE_ADDRESS", address: "abc.bru-2.zeebe.camunda.io", envVar: true, want: "abc.bru-2.zeebe.camunda.io"},
	} {
		s.Run(tc.name, func() {
			env.unset(InsecureEnvVar)
			env.unset(GatewayAddressEnvVar)
			config := &ClientConfig{UsePlaintextConnection: tc.plaintext, GatewayAddress: tc.address}
			if tc.envVar {
				config.GatewayAddress = "overridden"
				env.set(GatewayAddressEnvVar, tc.address)
			}

			client, err := NewClient(config)

			s.Require().NoError(err)
			s.Equal(tc.want, config.GatewayAddress)
			_ = client.Close()
		})
	}
}

func createSecureServer(withSan bool) (net.Listener, *grpc.Server) {
	certFile := "testdata/chain.cert.pem"
	keyFile := "testdata/private.key.pem"

	if withSan {
		certFile = "testdata/chain.cert.san.pem"
		keyFile = "testdata/private.key.san.pem"
	}

	creds, _ := credentials.NewServerTLSFromFile(certFile, keyFile)
	return createServerWithDefaultAddress(grpc.Creds(creds))
}

func createServerWithDefaultAddress(opts ...grpc.ServerOption) (net.Listener, *grpc.Server) {
	return createServer("0.0.0.0", "0", opts...)
}

func createServer(address string, port string, opts ...grpc.ServerOption) (net.Listener, *grpc.Server) {
	lis, _ := net.Listen("tcp", fmt.Sprintf("%s:%s", address, port))
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterGatewayServer(grpcServer, &pb.UnimplementedGatewayServer{})
	return lis, grpcServer
}
