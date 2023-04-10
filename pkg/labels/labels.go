// Copyright 2017 The Prometheus Authors
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

package labels

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/prometheus/common/model"
)

// IsValidName validates that model.LabelName is not a whitespace string and contains only valid UTF-8 symbols
func IsValidName(ln model.LabelName) bool {
	lns := string(ln)
	allSpaces := true
	for _, i := range lns {
		if !unicode.IsSpace(i) {
			allSpaces = false
			break
		}
	}
	return !allSpaces && utf8.ValidString(lns)
}

// IsValidSet validates that model.LabelSet keys and values are are valid
func IsValidSet(ls model.LabelSet) error {
	for ln, lv := range ls {
		if !IsValidName(ln) {
			return fmt.Errorf("invalid name %q", ln)
		}
		if !lv.IsValid() {
			return fmt.Errorf("invalid value %q", lv)
		}
	}
	return nil
}
