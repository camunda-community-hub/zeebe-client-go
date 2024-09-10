[![Community Extension](https://img.shields.io/badge/Community%20Extension-An%20open%20source%20community%20maintained%20project-FF4700)](https://github.com/camunda-community-hub/community)
[![](https://img.shields.io/badge/Lifecycle-Incubating-blue)](https://github.com/Camunda-Community-Hub/community/blob/main/extension-lifecycle.md#incubating-)

# Zeebe Go Client

The Zeebe Go client is a Go wrapper implementation around the GRPC (https://github.com/grpc/grpc) generated Zeebe client. It makes it possible to communicate with Zeebe Broker via the GRPC protocol, see the [Zeebe documentation](https://docs.zeebe.io/) for more information about the Zeebe project.

# Development

If we had a gateway-protocol change we need to make sure that we regenerate the protobuf file, which is used by the go client.

## Testing

### gRPC Mock

To regenerate the gateway mock `internal/mock_pb/mock_gateway.go` run [`mockgen`](https://github.com/golang/mock#installation) from the root of this project:

```
mockgen -source=pkg/pb/gateway.pb.go GatewayClient,Gateway_ActivateJobsClient > internal/mock_pb/mock_gateway.go
```

### Integration tests

Integration tests run zeebe in a container to test the client against. The version of Zeebe used is managed via 
a constant in [`internal/containersuite/containerSuite.go`](internal/containersuite/containerSuite.go#L36).

To add new zbctl tests, you must generate a golden file with the expected output of the command you are testing. The tests ignore numbers so you can leave any keys or timestamps in your golden file, even though these will most likely be different from test command's output. However, non-numeric variables are not ignored. For instance, the help menu contains:

```
--clientCache string    Specify the path to use for the OAuth credentials cache. If omitted, will read from the environment variable 'ZEEBE_CLIENT_CONFIG_PATH' (default "YOUR_HOME/.camunda/credentials")
```

To make them host-independent, the tests replace the HOME environment variable with `/tmp` which means you must do the same in your golden file.

## Dependencies

After making changes to the Go client, you can vendor the new dependencies with:

```
go mod vendor
```

This command will also remove or download dependencies as needed. To do that without vendoring them, you can run `go mod tidy`.
