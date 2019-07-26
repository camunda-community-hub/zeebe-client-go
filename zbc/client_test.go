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
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/zeebe-io/zeebe/clients/go/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"net"
	"strings"
	"testing"
)

func TestNewZBClientWithTls(t *testing.T) {
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, e := NewZBClient(&ZBClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "../resources/ca.cert.pem",
	})

	require.NoError(t, e)

	_, err := client.NewTopologyCommand().Send()

	require.Error(t, err)
	if status, ok := status.FromError(err); ok {
		require.Equal(t, codes.Unimplemented, status.Code())
	}
}

func TestNewZBClientWithoutTls(t *testing.T) {
	lis, _ := net.Listen("tcp", "0.0.0.0:0")

	grpcServer := grpc.NewServer()
	pb.RegisterGatewayServer(grpcServer, &pb.UnimplementedGatewayServer{})

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, e := NewZBClient(&ZBClientConfig{
		GatewayAddress:         fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		UsePlaintextConnection: true,
		CaCertificatePath:      "../resources/ca.cert.pem",
	})

	require.NoError(t, e)

	_, err := client.NewTopologyCommand().Send()

	require.Error(t, err)
	if status, ok := status.FromError(err); ok {
		require.Equal(t, codes.Unimplemented, status.Code())
	}

}

func TestNewZBClientWithDefaultRootCa(t *testing.T) {
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	client, e := NewZBClient(&ZBClientConfig{
		GatewayAddress: fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
	})

	require.NoError(t, e)

	_, err := client.NewTopologyCommand().Send()

	require.Error(t, err)
	if status, ok := status.FromError(err); ok {
		// asserts that an attempt was made to validate the certificate (which fails because it's not installed)
		require.Contains(t, status.Message(), "certificate signed by unknown authority")
	}
}

func TestNewZBClientWithPathToNonExistingFile(t *testing.T) {
	lis, grpcServer := createSecureServer()

	go grpcServer.Serve(lis)
	defer func() {
		grpcServer.Stop()
		_ = lis.Close()
	}()

	parts := strings.Split(lis.Addr().String(), ":")
	_, e := NewZBClient(&ZBClientConfig{
		GatewayAddress:    fmt.Sprintf("0.0.0.0:%s", parts[len(parts)-1]),
		CaCertificatePath: "../resources/non.existing",
	})

	require.Equal(t, NonExistingFileError, e)
}

func createSecureServer() (net.Listener, *grpc.Server) {
	listener, _ := net.Listen("tcp", "0.0.0.0:0")
	creds, _ := credentials.NewServerTLSFromFile("../resources/chain.cert.pem", "../resources/private.key.pem")

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterGatewayServer(grpcServer, &pb.UnimplementedGatewayServer{})

	return listener, grpcServer
}
