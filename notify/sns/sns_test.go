// Copyright 2021 Prometheus Team
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

package sns

import (
	"github.com/go-kit/log"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

var logger = log.NewNopLogger()

func TestValidateAndTruncateMessage(t *testing.T) {
	sBuff := make([]byte, 257*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err := validateAndTruncateMessage(string(sBuff), 256*1024)
	require.True(t, isTruncated)
	require.NoError(t, err)
	require.NotEqual(t, sBuff, truncatedMessage)
	require.Equal(t, len(truncatedMessage), 256*1024)

	sBuff = make([]byte, 100)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err = validateAndTruncateMessage(string(sBuff), 100)
	require.False(t, isTruncated)
	require.NoError(t, err)
	require.Equal(t, string(sBuff), truncatedMessage)

	invalidUtf8String := "\xc3\x28"
	_, _, err = validateAndTruncateMessage(invalidUtf8String, 100)
	require.Error(t, err)
}

func TestValidateAndTruncateSubject(t *testing.T) {
	var modifiedReasons []string
	notTruncate := make([]rune, 100)
	for i := range notTruncate {
		notTruncate[i] = 'e'
	}
	truncatedMessage := validateAndTruncateSubject(logger, string(notTruncate), &modifiedReasons)
	require.NotEqual(t, notTruncate, truncatedMessage)
	require.Equal(t, 100, utf8.RuneCountInString(string(truncatedMessage)))

	willBeTruncate := make([]rune, 101)
	for i := range willBeTruncate {
		willBeTruncate[i] = 'e'
	}
	truncatedMessage = validateAndTruncateSubject(logger, string(willBeTruncate), &modifiedReasons)
	require.Equal(t, string(notTruncate), truncatedMessage)
	require.Equal(t, len(modifiedReasons), 1)
	require.Equal(t, "Subject: Error - subject has been truncated from 101 characters because it exceeds the 100 character size limit", modifiedReasons[0])

	invalidAsciiString := "\xc3\x28"
	truncatedMessage = validateAndTruncateSubject(logger, invalidAsciiString, &modifiedReasons)
	require.Equal(t, truncatedMessage, SubjectNotASCII)
	require.Equal(t, len(modifiedReasons), 2)
	require.Equal(t, "Subject: Error - contains non printable ASCII characters", modifiedReasons[1])
}
