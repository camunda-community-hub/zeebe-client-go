[![Community Extension](https://img.shields.io/badge/Community%20Extension-An%20open%20source%20community%20maintained%20project-FF4700)](https://github.com/camunda-community-hub/community)
[![](https://img.shields.io/badge/Lifecycle-Incubating-blue)](https://github.com/Camunda-Community-Hub/community/blob/main/extension-lifecycle.md#incubating-)

# Zeebe Go Client

The Zeebe Go client is a Go wrapper implementation around the GRPC (https://github.com/grpc/grpc) generated Zeebe client. It makes it possible to communicate with Zeebe Broker via the GRPC protocol, see the [Zeebe documentation](https://docs.camunda.io/docs/components/zeebe/zeebe-overview/) for more information about the Zeebe project.

Get started with the Zeebe Go Client using the [step-by-step guide](/get-started.md), or learn more about how the Zeebe Go client implements [job workers](/job-worker.md). 

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

## Bootstrapping

In Go code, instantiate the client as follows:

```go
package main

import (
    "context"
    "fmt"
    "github.com/camunda-community-hub/zeebe-client-go/v8/pkg/zbc"
)

func main() {
    credsProvider, err := zbc.NewOAuthCredentialsProvider(&zbc.OAuthProviderConfig{
        ClientID:     "clientId",
        ClientSecret: "clientSecret",
        Audience:     "zeebeAddress",
    })
    if err != nil {
        panic(err)
    }

    client, err := zbc.NewClient(&zbc.ClientConfig{
        GatewayAddress:      "zeebeAddress",
        CredentialsProvider: credsProvider,
    })
    if err != nil {
        panic(err)
    }


    ctx := context.Background()
    response, err := client.NewTopologyCommand().Send(ctx)
    if err != nil {
        panic(err)
    }

    fmt.Println(response.String())
}
```

Let's go over this code snippet line by line:

1. Create the credentials provider for the OAuth protocol. This is needed to authenticate your client.
2. Create the client by passing in the address of the cluster we want to connect to and the credentials provider from the step above.
3. Send a test request to verify the connection was established.

The values for these settings can be taken from the connection information on the **Client Credentials** page. Note that `clientSecret` is only visible when you create the client credentials.

Another (more compact) option is to pass in the connection settings via environment variables:

```bash
export ZEEBE_ADDRESS='[Zeebe API]'
export ZEEBE_CLIENT_ID='[Client ID]'
export ZEEBE_CLIENT_SECRET='[Client Secret]'
export ZEEBE_AUTHORIZATION_SERVER_URL='[OAuth API]'
```

When you create client credentials in Camunda 8, you have the option to download a file with the lines above filled out for you.

Given these environment variables, you can instantiate the client as follows:

```go
package main

import (
    "context"
    "fmt"
    "github.com/camunda-community-hub/zeebe-client-go/v8/pkg/zbc"
    "os"
)

func main() {
    client, err := zbc.NewClient(&zbc.ClientConfig{
        GatewayAddress: os.Getenv("ZEEBE_ADDRESS"),
    })
    if err != nil {
        panic(err)
    }

    ctx := context.Background()
    response, err := client.NewTopologyCommand().Send(ctx)
    if err != nil {
        panic(err)
    }

    fmt.Println(response.String())
}
```
