// Copyright 2022 Prometheus Team
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

package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/alecthomas/kingpin.v2"
)

func Test_TestReceivers_Error(t *testing.T) {
	ctx := context.Background()
	parseContext := kingpin.ParseContext{}

	t.Run("invalid alertmanager config", func(t *testing.T) {
		test := testReceiversCmd{
			configFile: "testdata/conf.bad.yml",
		}

		err := test.testReceivers(ctx, &parseContext)
		require.Error(t, err)
		require.Equal(t, ErrInvalidConfigFile.Error(), err.Error())
	})

	t.Run("invalid alert", func(t *testing.T) {
		test := testReceiversCmd{
			configFile: "testdata/conf.receiver.yml",
			alertFile: "testdata/conf.bad-alert.yml",
		}

		err := test.testReceivers(ctx, &parseContext)
		require.Error(t, err)
		require.Equal(t, ErrInvalidAlertFile.Error(), err.Error())
	})

	t.Run("no receivers", func(t *testing.T) {
		test := testReceiversCmd{
			configFile: "testdata/conf.good.yml",
		}

		err := test.testReceivers(ctx, &parseContext)
		require.Error(t, err)
		require.Equal(t, ErrNoReceivers.Error(), err.Error())
	})
}
