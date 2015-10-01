// Copyright 2015 The Prometheus Authors
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

package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Silence defines the representation of a silence definiton
// in the Prometheus eco-system.
type Silence struct {
	ID uint64 `json:"id,omitempty"`

	Matchers []struct {
		Name    LabelName `json:"name,omitempty"`
		Value   string    `json:"value,omitempty"`
		IsRegex bool      `json:"isRegex"`
	} `json:"matchers"`

	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`

	CreatedBy string `json:"createdBy"`
	Comment   string `json:"comment,omitempty"`
}

func (s *Silence) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, s); err != nil {
		return err
	}

	for _, m := range s.Matchers {
		if len(m.Name) == 0 {
			return fmt.Errorf("label name in matcher must not be empty")
		}
	}
	return nil
}
