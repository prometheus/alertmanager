// Copyright 2023 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jira

import (
	"encoding/json"
)

type templateFunc func(string) (string, error)

type issue struct {
	Key        string       `json:"key,omitempty"`
	Fields     *issueFields `json:"fields,omitempty"`
	Transition *idNameValue `json:"transition,omitempty"`
}

type issueFields struct {
	Description any           `json:"description"`
	Issuetype   *idNameValue  `json:"issuetype,omitempty"`
	Labels      []string      `json:"labels,omitempty"`
	Priority    *idNameValue  `json:"priority,omitempty"`
	Project     *issueProject `json:"project,omitempty"`
	Resolution  *idNameValue  `json:"resolution,omitempty"`
	Summary     string        `json:"summary"`
	Status      *issueStatus  `json:"status,omitempty"`

	Fields map[string]any `json:"-"`
}

type idNameValue struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type issueProject struct {
	Key string `json:"key"`
}

type issueStatus struct {
	Name           string `json:"name"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type issueSearch struct {
	Expand     []string `json:"expand"`
	Fields     []string `json:"fields"`
	JQL        string   `json:"jql"`
	MaxResults int      `json:"maxResults"`
	StartAt    int      `json:"startAt"`
}

type issueSearchResult struct {
	Total  int     `json:"total"`
	Issues []issue `json:"issues"`
}

type issueTransitions struct {
	Transitions []idNameValue `json:"transitions"`
}

// MarshalJSON merges the struct issueFields and issueFields.CustomField together.
func (i issueFields) MarshalJSON() ([]byte, error) {
	jsonFields := map[string]interface{}{
		"description": i.Description,
		"summary":     i.Summary,
	}

	if i.Issuetype != nil {
		jsonFields["issuetype"] = i.Issuetype
	}

	if i.Labels != nil {
		jsonFields["labels"] = i.Labels
	}

	if i.Priority != nil {
		jsonFields["priority"] = i.Priority
	}

	if i.Project != nil {
		jsonFields["project"] = i.Project
	}

	if i.Resolution != nil {
		jsonFields["resolution"] = i.Resolution
	}

	if i.Status != nil {
		jsonFields["status"] = i.Status
	}

	for key, field := range i.Fields {
		jsonFields[key] = field
	}

	return json.Marshal(jsonFields)
}
