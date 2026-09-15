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
	"bytes"
	"encoding/json"
	"maps"
)

// issue represents a Jira issue wrapper.
type issue struct {
	Key        string       `json:"key,omitempty"`
	Fields     *issueFields `json:"fields,omitempty"`
	Transition *idNameValue `json:"transition,omitempty"`
}

type issueFields struct {
	Description *jiraDescription `json:"description,omitempty"`
	Issuetype   *idNameValue     `json:"issuetype,omitempty"`
	Labels      []string         `json:"labels,omitempty"`
	Priority    *idNameValue     `json:"priority,omitempty"`
	Project     *issueProject    `json:"project,omitempty"`
	Resolution  *idNameValue     `json:"resolution,omitempty"`
	Summary     *string          `json:"summary,omitempty"`
	Status      *issueStatus     `json:"status,omitempty"`

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
	Fields     []string `json:"fields"`
	JQL        string   `json:"jql"`
	MaxResults int      `json:"maxResults"`
}

type issueSearchResult struct {
	Issues []issue `json:"issues"`
}

type issueTransitions struct {
	Transitions []idNameValue `json:"transitions"`
}

// MarshalJSON merges the struct issueFields and issueFields.CustomField together.
func (i issueFields) MarshalJSON() ([]byte, error) {
	jsonFields := map[string]any{}

	if i.Summary != nil {
		jsonFields["summary"] = *i.Summary
	}

	// Only include description when it has content.
	if i.Description != nil && !i.Description.IsEmpty() {
		jsonFields["description"] = i.Description
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

	// copy custom/unknown fields into the outgoing map
	if i.Fields != nil {
		maps.Copy(jsonFields, i.Fields)
	}

	return json.Marshal(jsonFields)
}

// jiraDescription holds either a plain string (v2 API) description or ADF (Atlassian Document Format) JSON (v3 API).
type jiraDescription struct {
	StringDescription  *string         // non-nil if the description is a simple string
	RawJSONDescription json.RawMessage // non-empty if the description is structured JSON
}

func (jd jiraDescription) MarshalJSON() ([]byte, error) {
	// If there's a structured JSON payload, return it as-is.
	if len(jd.RawJSONDescription) > 0 {
		out := make([]byte, len(jd.RawJSONDescription))
		copy(out, jd.RawJSONDescription)
		return out, nil
	}

	// If we have a string representation, let json.Marshal quote it properly.
	if jd.StringDescription != nil {
		return json.Marshal(*jd.StringDescription)
	}

	// No value: represent as JSON null.
	return []byte("null"), nil
}

func (jd *jiraDescription) UnmarshalJSON(data []byte) error {
	// Reset current state
	jd.StringDescription = nil
	jd.RawJSONDescription = nil

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		// nothing to do (leave both fields nil/empty)
		return nil
	}

	// If it starts with object or array token, treat as structured JSON and keep raw bytes.
	switch trimmed[0] {
	case '{', '[':
		// store a copy of the raw JSON
		jd.RawJSONDescription = append(json.RawMessage(nil), trimmed...)
		return nil
	default:
		// otherwise try to unmarshal as string (expected for Jira v2)
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			// fallback: if it's not a string but also not an object/array, keep raw bytes
			jd.RawJSONDescription = append(json.RawMessage(nil), trimmed...)
			return nil
		}
		jd.StringDescription = &s
		return nil
	}
}

// IsEmpty reports whether the jiraDescription contains no useful value.
func (jd *jiraDescription) IsEmpty() bool {
	if jd == nil {
		return true
	}
	return jd.StringDescription == nil && len(jd.RawJSONDescription) == 0
}
