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
	"github.com/zeebe-io/zeebe/clients/go/pb"
)

type Job struct {
	pb.ActivatedJob
}

func (job *Job) GetVariablesAsMap() (map[string]interface{}, error) {
	var variablesMap map[string]interface{}
	return variablesMap, job.GetVariablesAs(&variablesMap)
}

func (job *Job) GetVariablesAs(variablesType interface{}) error {
	return json.Unmarshal([]byte(job.Variables), variablesType)
}

func (job *Job) GetCustomHeadersAsMap() (map[string]interface{}, error) {
	var customHeadersMap map[string]interface{}
	return customHeadersMap, job.GetCustomHeadersAs(&customHeadersMap)
}

func (job *Job) GetCustomHeadersAs(customHeadersType interface{}) error {
	return json.Unmarshal([]byte(job.CustomHeaders), customHeadersType)
}
