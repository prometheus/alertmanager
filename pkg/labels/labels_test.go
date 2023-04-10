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
	"strings"
	"testing"
	"unicode"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestIsValidName(t *testing.T) {
	testCases := []struct {
		name      string
		labelName model.LabelName
		valid     bool
	}{
		{
			name:      "invalid: empty string",
			labelName: "",
			valid:     false,
		},
		{
			name:      "invalid: all spaces",
			labelName: " ",
			valid:     false,
		},
		{
			name: "invalid: only whitespaces",
			labelName: func() model.LabelName {
				whiteSpaceBuilder := strings.Builder{}
				for _, r16 := range unicode.White_Space.R16 {
					for sym := r16.Lo; sym <= r16.Hi; sym += r16.Stride {
						whiteSpaceBuilder.WriteRune(rune(sym))
					}
				}
				return model.LabelName(whiteSpaceBuilder.String())
			}(),
			valid: false,
		},
		{
			name:      "valid: Prometheus label",
			labelName: "TEST_label_name_12345",
			valid:     true,
		},
		{
			name:      "valid: any UTF-8 character",
			labelName: "\nlabel:test data.爱!🙂\t",
			valid:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.valid, IsValidName(testCase.labelName))
		})
	}
}
