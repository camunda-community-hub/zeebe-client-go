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
	"encoding/json"
	"fmt"
	"github.com/zeebe-io/zeebe/clients/go/pkg/zbc"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	DefaultAddressHost = "127.0.0.1"
	DefaultAddressPort = "26500"
	AddressEnvVar      = "ZEEBE_ADDRESS"
	defaultTimeout     = 10 * time.Second
)

var client zbc.Client

var addressFlag string
var caCertPathFlag string
var clientIDFlag string
var clientSecretFlag string
var audienceFlag string
var authzURLFlag string
var insecureFlag bool
var clientCacheFlag string

var rootCmd = &cobra.Command{
	Use:   "zbctl",
	Short: "zeebe command line interface",
	Long: `zbctl is command line interface designed to create and read resources inside zeebe broker.
It is designed for regular maintenance jobs such as:
	* deploying workflows,
	* creating jobs and workflow instances
	* activating, completing or failing jobs
	* update variables and retries
	* view cluster status`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&addressFlag, "address", "", "Specify a contact point address. If omitted, will read from the environment variable '"+AddressEnvVar+"' (default '"+fmt.Sprintf("%s:%s", DefaultAddressHost, DefaultAddressPort)+"')")
	rootCmd.PersistentFlags().StringVar(&caCertPathFlag, "certPath", "", "Specify a path to a certificate with which to validate gateway requests. If omitted, will read from the environment variable '"+zbc.CaCertificatePath+"'")
	rootCmd.PersistentFlags().StringVar(&clientIDFlag, "clientId", "", "Specify a client identifier to request an access token. If omitted, will read from the environment variable '"+zbc.OAuthClientIdEnvVar+"'")
	rootCmd.PersistentFlags().StringVar(&clientSecretFlag, "clientSecret", "", "Specify a client secret to request an access token. If omitted, will read from the environment variable '"+zbc.OAuthClientSecretEnvVar+"'")
	rootCmd.PersistentFlags().StringVar(&audienceFlag, "audience", "", "Specify the resource that the access token should be valid for. If omitted, will read from the environment variable '"+zbc.OAuthTokenAudienceEnvVar+"'")
	rootCmd.PersistentFlags().StringVar(&authzURLFlag, "authzUrl", zbc.OAuthDefaultAuthzURL, "Specify an authorization server URL from which to request an access token. If omitted, will read from the environment variable '"+zbc.OAuthAuthorizationUrlEnvVar+"'")
	rootCmd.PersistentFlags().BoolVar(&insecureFlag, "insecure", false, "Specify if zbctl should use an unsecured connection. If omitted, will read from the environment variable '"+zbc.InsecureEnvVar+"'")
	rootCmd.PersistentFlags().StringVar(&clientCacheFlag, "clientCache", zbc.DefaultOauthYamlCachePath, "Specify the path to use for the OAuth credentials cache. If omitted, will read from the environment variable '"+zbc.OAuthCachePathEnvVar+"'")
}

// initClient will create a client with in the following precedence: flag, environment variable, default
var initClient = func(cmd *cobra.Command, args []string) error {
	var err error
	var credsProvider zbc.CredentialsProvider

	host, port := parseAddress()

	// override env vars with CLI parameters, if any
	if err := setSecurityParamsAsEnv(); err != nil {
		return err
	}

	_, idExists := os.LookupEnv(zbc.OAuthClientIdEnvVar)
	_, secretExists := os.LookupEnv(zbc.OAuthClientSecretEnvVar)

	if idExists || secretExists {
		_, audienceExists := os.LookupEnv(zbc.OAuthTokenAudienceEnvVar)
		if !audienceExists {
			if err := os.Setenv(zbc.OAuthTokenAudienceEnvVar, host); err != nil {
				return err
			}
		}

		providerConfig := zbc.OAuthProviderConfig{}

		// create a credentials provider with the specified parameters
		credsProvider, err = zbc.NewOAuthCredentialsProvider(&providerConfig)

		if err != nil {
			return err
		}
	}

	client, err = zbc.NewClient(&zbc.ClientConfig{
		GatewayAddress:      fmt.Sprintf("%s:%s", host, port),
		CredentialsProvider: credsProvider,
	})
	return err
}

func setSecurityParamsAsEnv() (err error) {
	setEnv := func(envVar, value string) {
		if err == nil {
			err = os.Setenv(envVar, value)
		}
	}

	if insecureFlag {
		setEnv(zbc.InsecureEnvVar, "true")
	}
	if caCertPathFlag != "" {
		setEnv(zbc.CaCertificatePath, caCertPathFlag)
	}
	if clientIDFlag != "" {
		setEnv(zbc.OAuthClientIdEnvVar, clientIDFlag)
	}
	if clientSecretFlag != "" {
		setEnv(zbc.OAuthClientSecretEnvVar, clientSecretFlag)
	}
	if audienceFlag != "" {
		setEnv(zbc.OAuthTokenAudienceEnvVar, audienceFlag)
	}
	if shouldOverwriteEnvVar("authzUrl", zbc.OAuthAuthorizationUrlEnvVar) {
		setEnv(zbc.OAuthAuthorizationUrlEnvVar, authzURLFlag)
	}
	if shouldOverwriteEnvVar("clientCache", zbc.DefaultOauthYamlCachePath) {
		setEnv(zbc.OAuthCachePathEnvVar, clientCacheFlag)
	}

	return
}

// decides whether to overwrite env var (for parameters with default values)
func shouldOverwriteEnvVar(cliParam, envVar string) bool {
	cliParameterSet := rootCmd.Flags().Changed(cliParam)
	_, exists := os.LookupEnv(envVar)
	return cliParameterSet || !exists
}

func parseAddress() (address string, port string) {
	address = DefaultAddressHost
	port = DefaultAddressPort

	if len(addressFlag) > 0 {
		address = addressFlag
	} else if addressEnv, exists := os.LookupEnv(AddressEnvVar); exists {
		address = addressEnv

	}

	if strings.Contains(address, ":") {
		parts := strings.Split(address, ":")
		address = parts[0]
		port = parts[1]
	}

	return
}

func keyArg(key *int64) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("expects key as only positional argument")
		}

		value, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid argument %q for %q: %s", args[0], "key", err)
		}

		*key = value

		return nil
	}
}

func printJson(value interface{}) error {
	valueJson, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		fmt.Println(string(valueJson))
	}
	return err
}
