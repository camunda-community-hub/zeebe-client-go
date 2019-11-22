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

package entities

import (
	"encoding/json"

	"github.com/zeebe-io/zeebe/clients/go/pkg/pb"
)

// Job represents a single work item of a workflow.
//
// See https://docs.zeebe.io/basics/job-workers.html#what-is-a-job for details
// on jobs.
type Job struct {
	pb.ActivatedJob
}

// GetVariablesAsMap retuns a map of a workflow instance's variables.
//
// See https://docs.zeebe.io/reference/variables.html for details on workflow
// variables.
func (j *Job) GetVariablesAsMap() (map[string]interface{}, error) {
	var m map[string]interface{}
	return m, j.GetVariablesAs(&m)
}

// GetVariablesAs unmarshals the JSON representation of a workflow instance's
// variables into type t.
//
// See https://docs.zeebe.io/reference/variables.html for details on workflow
// variables.
func (j *Job) GetVariablesAs(t interface{}) error {
	return json.Unmarshal([]byte(j.Variables), t)
}

// GetCustomHeadersAsMap returns a map of a workflow's custom headers.
//
// Unlike variables, custom headers are specific to a workflow, as opposed to a
// workflow instance.
func (j *Job) GetCustomHeadersAsMap() (map[string]string, error) {
	var m map[string]string
	return m, j.GetCustomHeadersAs(&m)
}

// GetCustomHeadersAs unmarshals the JSON representation of a workflow's
// custom headers into type t.
//
// Unlike variables, custom headers are specific to a workflow, as opposed to a
// workflow instance.
func (j *Job) GetCustomHeadersAs(t interface{}) error {
	return json.Unmarshal([]byte(j.CustomHeaders), t)
}
