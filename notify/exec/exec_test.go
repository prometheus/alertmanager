// Copyright 2019 Prometheus Team
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

package exec

import (
	"context"
	"flag"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

const updateFlag = "update"

func init() {
	if f := flag.Lookup(updateFlag); f != nil {
		getter, ok := f.Value.(flag.Getter)
		if !ok {
			panic("existing -update flag is not a Getter")
		}

		_, ok = getter.Get().(bool)
		if !ok {
			panic("existing -update flag does not provide boolean values")
		}
	} else {
		flag.Bool(updateFlag, false, "update test fixtures")
	}
}

func updateFixtures() bool {
	return flag.Lookup(updateFlag).Value.(flag.Getter).Get().(bool)
}

func newConfig(t testing.TB, fixtureAssertion string, argv ...string) *config.ExecConfig {
	var (
		arguments []string
		filePath  string
		execFile  string
		err       error
	)

	if fixtureAssertion != "" {
		filePath = filepath.Join("testdata", fixtureAssertion)
		filePath, err = filepath.Abs(filePath)
		require.NoError(t, err)

		if updateFixtures() && !strings.HasPrefix(fixtureAssertion, "bad_") {
			arguments = append(arguments, "-f", filePath)
		}
	} else {
		filePath = filepath.Join("testdata", "404.json")
		filePath, err = filepath.Abs(filePath)
		require.NoError(t, err)
	}

	arguments = append(arguments, argv...)
	arguments = append(arguments, filePath)
	execFile = filepath.Join("testdata", "test-exec.sh")
	execFile, err = filepath.Abs(execFile)
	require.NoError(t, err)

	return &config.ExecConfig{
		ExecFile:  execFile,
		Arguments: arguments,
	}
}

func newSubject(t testing.TB, conf *config.ExecConfig) *Notifier {
	logger := promslog.NewNopLogger()
	tmpl := test.CreateTmpl(t)
	notifier, err := New(conf, tmpl, logger)
	require.NoError(t, err)

	return notifier
}

func TestExecSuccess(t *testing.T) {
	conf := newConfig(t, "success.json")
	notifier := newSubject(t, conf)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"Message": "message",
			},
			StartsAt: time.Unix(0, 0),
		},
	}

	retry, err := notifier.Notify(ctx, alert)
	require.NoError(t, err)
	require.Equal(t, false, retry)
}

func TestExecRetry(t *testing.T) {
	conf := newConfig(t, "")
	notifier := newSubject(t, conf)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert := &types.Alert{}

	retry, err := notifier.Notify(ctx, alert)
	require.Error(t, err)
	require.Equal(t, true, retry)
}

func TestExecError(t *testing.T) {
	conf := newConfig(t, "bad_error.json")
	notifier := newSubject(t, conf)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	retry, err := notifier.Notify(ctx)
	require.Error(t, err)
	require.Equal(t, false, retry)
}

func TestExecTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("not testing timeouts in short mode")
	}

	conf := newConfig(t, "timeout.json", "-s", "5")
	conf.Timeout = 2 * time.Second
	notifier := newSubject(t, conf)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	retry, err := notifier.Notify(ctx)
	require.Error(t, err)
	require.Equal(t, false, retry)
}

func TestExecScope(t *testing.T) {
	conf := newConfig(t, "scope.json",
		"-e", "FOO=BAR",
		"-e", "GAFF=BAC",
		"-e", "SECRET=DEADBEEFCAFE",
		"-d", "./testdata",
	)
	conf.Environment = map[string]string{
		"FOO":  "BAR",
		"GAFF": "BAC",
	}
	conf.EnvironmentFiles = map[string]string{
		"SECRET": "testdata/secret.env",
	}
	conf.WorkingDir = "./testdata"
	conf.ExecFile = filepath.Join("testdata", "assert-scope.sh")
	notifier := newSubject(t, conf)

	rootDir, err := filepath.Abs(".")
	require.NoError(t, err)
	conf.SetDirectory(rootDir)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	retry, err := notifier.Notify(ctx)
	require.NoError(t, err)
	require.Equal(t, false, retry)
}
