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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/camunda/zeebe/clients/go/v8/internal/containersuite"
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/suite"
)

var zbctl string

const (
	// NOTE: taken from https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
	semVer = `(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*` +
		`|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`
)

type integrationTestSuite struct {
	*containersuite.ContainerSuite
}

type testCase struct {
	name           string
	setupCmds      [][]string
	envVars        []string
	cmd            []string
	goldenFile     string
	jsonOutput     bool
	useHostAndPort bool
}

var tests = []testCase{
	{
		name:       "print help",
		cmd:        []string{"help"},
		envVars:    []string{"HOME=/tmp"},
		goldenFile: "testdata/help.golden",
	},
	{
		name:       "print version",
		cmd:        []string{"version"},
		envVars:    []string{"HOME=/tmp"},
		goldenFile: "testdata/version.golden",
	},
	{
		name:       "missing insecure flag",
		cmd:        []string{"status"},
		envVars:    []string{"HOME=/tmp"},
		goldenFile: "testdata/without_insecure.golden",
	},
	{
		name: "using insecure env var",
		cmd:  []string{"status"},
		// we need to set the path so it evaluates $HOME before we overwrite it
		envVars:    []string{fmt.Sprintf("%s=true", zbc.InsecureEnvVar), fmt.Sprintf("PATH=%s", os.Getenv("PATH"))},
		goldenFile: "testdata/topology.golden",
	},
	{
		name: "using json flag",
		cmd:  strings.Fields("status --output=json"),
		// we need to set the path so it evaluates $HOME before we overwrite it
		envVars:    []string{fmt.Sprintf("%s=true", zbc.InsecureEnvVar), fmt.Sprintf("PATH=%s", os.Getenv("PATH"))},
		goldenFile: "testdata/topology_json.golden",
		jsonOutput: true,
	},
	{
		name: "using json flag and host and port arguments",
		cmd:  strings.Fields("status --output=json"),
		// we need to set the path so it evaluates $HOME before we overwrite it
		envVars:        []string{fmt.Sprintf("%s=true", zbc.InsecureEnvVar), fmt.Sprintf("PATH=%s", os.Getenv("PATH"))},
		goldenFile:     "testdata/topology_json.golden",
		jsonOutput:     true,
		useHostAndPort: true,
	},
	{
		name:       "deploy processes",
		cmd:        strings.Fields("--insecure deploy testdata/model.bpmn testdata/job_model.bpmn --resourceNames=model.bpmn,job.bpmn"),
		goldenFile: "testdata/deploy.golden",
		jsonOutput: true,
	},
	{
		name:       "deploy resources",
		cmd:        strings.Fields("--insecure deploy resource testdata/model.bpmn testdata/drg-force-user.dmn --resourceNames=model.bpmn,drg-force-user.dmn"),
		goldenFile: "testdata/deploy_resources.golden",
		jsonOutput: true,
	},
	{
		name:       "create instance with process id",
		setupCmds:  [][]string{strings.Fields("--insecure deploy testdata/model.bpmn")},
		cmd:        strings.Fields("--insecure create instance process"),
		goldenFile: "testdata/create_instance.golden",
		jsonOutput: true,
	},
	{
		name:       "create instance with process key",
		setupCmds:  [][]string{strings.Fields("--insecure deploy testdata/model.bpmn")},
		cmd:        strings.Fields("--insecure create instance 2251799813685249"),
		goldenFile: "testdata/create_instance.golden",
		jsonOutput: true,
	},
	{
		name:       "evaluate decision with decision id",
		setupCmds:  [][]string{strings.Fields("--insecure deploy resource testdata/drg-force-user.dmn")},
		cmd:        strings.Fields("--insecure evaluate decision jedi_or_sith --variables testdata/decision_variables.json"),
		goldenFile: "testdata/evaluate_decision.golden",
		jsonOutput: true,
	},
	{
		name:       "evaluate decision with decision key",
		setupCmds:  [][]string{strings.Fields("--insecure deploy resource testdata/drg-force-user.dmn")},
		cmd:        strings.Fields("--insecure evaluate decision 2251799813685270 --variables {\"lightsaberColor\":\"blue\"}"),
		goldenFile: "testdata/evaluate_decision.golden",
		jsonOutput: true,
	},
	{
		name: "create worker",
		setupCmds: [][]string{
			strings.Fields("--insecure deploy testdata/job_model.bpmn"),
			strings.Fields("--insecure create instance jobProcess"),
		},
		cmd:        strings.Fields("create --insecure worker jobType --maxJobsHandle 1 --handler echo"),
		goldenFile: "testdata/create_worker.golden",
	},
	{
		name:       "empty activate job",
		cmd:        strings.Fields("--insecure activate jobs jobType --maxJobsToActivate 0"),
		goldenFile: "testdata/empty_activate_job.golden",
		jsonOutput: true,
	},
	{
		name: "single activate job",
		setupCmds: [][]string{
			strings.Fields("--insecure deploy testdata/job_model.bpmn"),
			strings.Fields("--insecure create instance jobProcess"),
		},
		cmd:        strings.Fields("--insecure activate jobs jobType --maxJobsToActivate 1"),
		goldenFile: "testdata/single_activate_job.golden",
		jsonOutput: true,
	},
	{
		name: "double activate job",
		setupCmds: [][]string{
			strings.Fields("--insecure deploy testdata/job_model.bpmn"),
			strings.Fields("--insecure create instance jobProcess"),
			strings.Fields("--insecure create instance jobProcess"),
		},
		cmd:        strings.Fields("--insecure activate jobs jobType --maxJobsToActivate 2"),
		goldenFile: "testdata/double_activate_job.golden",
		jsonOutput: true,
	},
	{
		name:       "send message with a space and json string as variables",
		setupCmds:  [][]string{strings.Fields("--insecure deploy testdata/start_event.bpmn")},
		cmd:        []string{"--insecure", "publish", "message", "Start Process", "--correlationKey", "1234", "--variables", "{\"FOO\":\"BAR\"}"},
		goldenFile: "testdata/publish_message_with_space.golden",
		jsonOutput: true,
	},
	{
		name:       "send message with a json file as variables",
		cmd:        []string{"--insecure", "publish", "message", "Start Process", "--correlationKey", "1234", "--variables", "testdata/message_variables.json"},
		goldenFile: "testdata/publish_message_with_variables_file.golden",
		jsonOutput: true,
	},
	{
		name:       "broadcast signal with a space and json string as variables",
		setupCmds:  [][]string{strings.Fields("--insecure deploy testdata/signal_start_event.bpmn")},
		cmd:        []string{"--insecure", "broadcast", "signal", "Start Process", "--variables", "{\"FOO\":\"BAR\"}"},
		goldenFile: "testdata/broadcast_signal_with_space.golden",
		jsonOutput: true,
	},
	{
		name:       "broadcast signal with a json file as variables",
		cmd:        []string{"--insecure", "broadcast", "signal", "Start Process", "--variables", "testdata/signal_variables.json"},
		goldenFile: "testdata/broadcast_signal_with_variables_file.golden",
		jsonOutput: true,
	},
}

func TestZbctlWithInsecureGateway(t *testing.T) {
	output, err := buildZbctl()
	if err != nil {
		fmt.Println(string(output))
		t.Fatal(fmt.Errorf("couldn't build zbctl: %w", err))
	}

	suite.Run(t,
		&integrationTestSuite{
			ContainerSuite: &containersuite.ContainerSuite{
				WaitTime:       time.Second,
				ContainerImage: "camunda/zeebe:current-test",
				Env: map[string]string{
					"ZEEBE_BROKER_GATEWAY_LONGPOLLING_ENABLED": "false",
				},
			},
		})
}

func (s *integrationTestSuite) AfterTest(_, _ string) {
	// overload to ignore the parent behavior; we instead print the container logs after each failed
	// test and not at the very end of all test cases, where it's more difficult to know what went
	// wrong; if you add more top level tests, like TestCommonCommands, make sure to also add a block
	// of code in case the test fails to print out the container logs using s.PrintFailedContainerLogs()
}

func (s *integrationTestSuite) TestCommonCommands() {
	for _, test := range tests {
		passed := s.T().Run(test.name, func(t *testing.T) {
			for _, cmd := range test.setupCmds {
				if cmdOut, err := s.runCommand(cmd, false); err != nil {
					t.Fatalf("failed while executing set up command '%s' (%v). Output: \n%s",
						strings.Join(cmd, " "), err, cmdOut)
				}
			}

			setupCmdsCount := len(test.setupCmds)
			if setupCmdsCount > 0 {
				// mitigates race condition between setup commands and test command, wait 1 second
				// per setup command
				<-time.After(time.Duration(setupCmdsCount) * time.Second)
			}

			cmdOut, err := s.runCommand(test.cmd, test.useHostAndPort, test.envVars...)
			exitErr := &exec.ExitError{}
			// if the exit code is -1, then the process did not start properly or was killed by a
			// signal, which is what happens when its execution times out. as some tests rely
			// on exiting with non 0 status codes, we can't just bail out if there is some error
			if err != nil && errors.As(err, &exitErr) && exitErr.ExitCode() == -1 {
				t.Fatalf("failed while executing command under test '%s' (%v). Output: \n%s",
					strings.Join(test.cmd, " "), err, cmdOut)
			}

			goldenOut, err := os.ReadFile(test.goldenFile)
			if err != nil {
				t.Fatal(err)
			}

			if test.jsonOutput {
				fmtJSON, err := reformatJSON(cmdOut)
				if err != nil {
					t.Fatalf("failed to reformat response JSON: %v\nErroneous JSON: %s", err, cmdOut)
				}
				cmdOut = fmtJSON

				fmtGolden, err := reformatJSON(goldenOut)
				if err != nil {
					t.Fatalf("failed to reformat golden JSON: %v\nErroneous JSON: %s", err, goldenOut)
				}
				goldenOut = fmtGolden
			}

			assertEq(t, test, goldenOut, cmdOut)
		})

		if !passed {
			_, _ = fmt.Fprintln(os.Stderr, "=====================================")
			_, _ = fmt.Fprintf(os.Stderr, "Test case '%s' failed, printing container logs up until now\n", test.name)
			s.PrintFailedContainerLogs()
		}
	}
}

// reformatJSON formats the JSON files in the same way so that whitespace differences are ignored
func reformatJSON(in []byte) ([]byte, error) {
	buf := &bytes.Buffer{}

	// must compact before calling json.Indent because event though that will remove leading whitespace, it won't remove
	// trailing whitespace.
	if err := json.Compact(buf, in); err != nil {
		return nil, err
	}

	compacted := make([]byte, buf.Len())
	copy(compacted, buf.Bytes())
	buf.Reset()

	if err := json.Indent(buf, compacted, "", "\t"); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func assertEq(t *testing.T, test testCase, golden, cmdOut []byte) {
	wantLines := strings.Split(string(golden), "\n")
	gotLines := strings.Split(string(cmdOut), "\n")

	opt := composeComparers(cmpIgnoreVersion, cmpIgnoreNums)
	if diff := cmp.Diff(wantLines, gotLines, opt); diff != "" {
		t.Fatalf("%s: diff (-want +got):\n%s", test.name, diff)
	}
}

func composeComparers(cmpFuncs ...func(x string, y string) bool) cmp.Option {
	return cmp.Comparer(func(x, y string) bool {
		for _, cmpFunc := range cmpFuncs {
			if cmpFunc(x, y) {
				return true
			}
		}

		return false
	})
}

func cmpIgnoreVersion(x, y string) bool {
	versionRegex := regexp.MustCompile(semVer)
	newX := versionRegex.ReplaceAllString(x, "")
	newY := versionRegex.ReplaceAllString(y, "")

	return newX == newY
}

func cmpIgnoreNums(x, y string) bool {
	numbersRegex := regexp.MustCompile(`\d+`)
	newX := numbersRegex.ReplaceAllString(x, "")
	newY := numbersRegex.ReplaceAllString(y, "")

	return newX == newY
}

// runCommand runs the zbctl command and returns the combined output from stdout and stderr
func (s *integrationTestSuite) runCommand(command []string, useHostAndPort bool, envVars ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	args := command
	args = append(args, "--requestTimeout", "10s")
	if useHostAndPort {
		args = append(args, "--host", s.GatewayHost)
		args = append(args, "--port", fmt.Sprint(s.GatewayPort))
	} else {
		args = append(args, "--address", s.GatewayAddress)
	}
	cmd := exec.CommandContext(ctx, fmt.Sprintf("./dist/%s", zbctl), args...)

	cmd.Env = append(cmd.Env, envVars...)
	output, err := cmd.CombinedOutput()
	return output, err
}

func buildZbctl() ([]byte, error) {
	switch runtime.GOOS {
	case "linux":
		zbctl = "zbctl"
	case "darwin":
		zbctl = "zbctl.darwin"
	default:
		return nil, fmt.Errorf("can't run zbctl tests on unsupported OS '%s'", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// we need to build all binaries, because this is run after the go build stage on CI and will overwrite the binaries
	cmd := exec.CommandContext(ctx, "./build.sh", runtime.GOOS)
	cmd.Env = append(os.Environ(), "RELEASE_VERSION=release-test", "RELEASE_HASH=1234567890")
	fmt.Printf("Building zbctl before running the tests with command: %s\n", cmd.String())

	return cmd.CombinedOutput()
}
