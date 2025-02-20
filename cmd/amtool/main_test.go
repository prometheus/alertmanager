// Copyright 2018 Prometheus Team
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

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptyCommand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		exitCode int
		stdout   string
		stderr   string
	}{
		{
			name:     "help without command",
			args:     []string{},
			exitCode: 0,
			stdout:   `usage: amtool [<flags>] <command> [<args> ...]`,
		},
		{
			name:     "empty command",
			args:     []string{""},
			exitCode: 1,
			stderr:   `error: expected command but got ""`,
		},
		{
			name:   "version",
			args:   []string{"--version"},
			stdout: fmt.Sprintf("amtool, version  (branch: , revision: unknown)\n  build user:       \n  build date:       \n  go version:       %s\n  platform:         %s/%s\n  tags:             unknown\n", runtime.Version(), runtime.GOOS, runtime.GOARCH),
		},
		// In this test, we a running a command that invokes the --verbose flag.
		// The test expects that there is no running instance on port 1.
		{
			name:     "verbose flag",
			exitCode: 1,
			args:     []string{"--verbose", "alert", "add", "--alertmanager.url=http://localhost:1"},
			stderr:   `error: Post "http://localhost:1/api/v2/alerts": dial tcp 127.0.0.1:1`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if os.Getenv("FORK") == "1" {
				run(append([]string{"amtool"}, tc.args...))
			}

			stdout, stderr, err := RunForkTest(t)

			if tc.exitCode == 0 {
				require.NoError(t, err, "STDOUT:\n%s\n\nSTDERR:\n%s", stdout, stderr)
			} else {
				require.EqualError(t, err, fmt.Sprintf("exit status %d", tc.exitCode), "STDOUT:\n%s\n\nSTDERR:\n%s", stdout, stderr)
			}

			if tc.stderr == "" {
				require.Empty(t, stderr)
			} else {
				require.Contains(t, stderr, tc.stderr)
			}

			if tc.stdout == "" {
				require.Empty(t, stdout)
			} else {
				require.Contains(t, stdout, tc.stdout)
			}
		})
	}
}

func RunForkTest(t *testing.T) (string, string, error) {
	t.Helper()

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%v", t.Name()))
	cmd.Env = append(os.Environ(), "FORK=1")

	var stdoutB, stderrB bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = &stdoutB
	cmd.Stderr = &stderrB

	err := cmd.Run()

	return stdoutB.String(), stderrB.String(), err
}
