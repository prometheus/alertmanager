// Copyright The Prometheus Authors
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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientOptions_Validate(t *testing.T) {
	cases := []struct {
		name    string
		opts    ClientOptions
		wantErr bool
	}{
		{
			name: "minimal valid",
			opts: ClientOptions{Brokers: []string{"a:9092"}},
		},
		{
			name: "fully populated valid",
			opts: ClientOptions{
				Brokers:     []string{"a:9092", "b:9092"},
				Topic:       "t",
				ClientID:    "test",
				Acks:        AcksAll,
				Compression: CompressionZstd,
			},
		},
		{
			name:    "no brokers",
			opts:    ClientOptions{},
			wantErr: true,
		},
		{
			name:    "empty broker entry",
			opts:    ClientOptions{Brokers: []string{"a:9092", ""}},
			wantErr: true,
		},
		{
			name: "bad acks",
			opts: ClientOptions{
				Brokers: []string{"a:9092"},
				Acks:    "majority",
			},
			wantErr: true,
		},
		{
			name: "bad compression",
			opts: ClientOptions{
				Brokers:     []string{"a:9092"},
				Compression: "deflate",
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateFormat(t *testing.T) {
	require.NoError(t, ValidateFormat(FormatJSON))
	require.NoError(t, ValidateFormat(FormatProtobuf))
	require.Error(t, ValidateFormat(""))
	require.Error(t, ValidateFormat("yaml"))
}
