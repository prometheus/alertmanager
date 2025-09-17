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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitRetry   = 3
)

const messageVersion = "1"

// Message defines the JSON object send to executables.
type Message struct {
	*template.Data

	// The protocol version.
	Version  string `json:"version"`
	GroupKey string `json:"groupKey"`
}

// Notifier implements a Notifier for Exec notifications.
type Notifier struct {
	conf   *config.ExecConfig
	tmpl   *template.Template
	logger *slog.Logger
	env    []string
}

// New returns a new Exec notifier.
func New(c *config.ExecConfig, t *template.Template, l *slog.Logger) (*Notifier, error) {
	env := make([]string, len(c.Environment))
	for k, v := range c.Environment {
		env = append(env, k+"="+v)
	}

	for k, f := range c.EnvironmentFiles {
		v, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}

		v = bytes.TrimSpace(v)
		env = append(env, k+"="+string(v))
	}

	return &Notifier{conf: c, tmpl: t, logger: l, env: env}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var (
		err    error
		cmd    *exec.Cmd
		cmdCtx context.Context = ctx
	)

	if n.conf.Timeout > 0 {
		var cancel func()
		cmdCtx, cancel = context.WithTimeout(ctx, n.conf.Timeout)

		defer cancel()
	}

	cmd = exec.CommandContext(cmdCtx, n.conf.ExecFile, n.conf.Arguments...)
	cmd.Dir = n.conf.WorkingDir
	cmd.Env = append(os.Environ(), n.env...)

	var (
		stdin  io.WriteCloser
		stdout io.ReadCloser
		stderr io.ReadCloser
	)

	stdin, err = cmd.StdinPipe()
	if err != nil {
		return false, fmt.Errorf("failed to open STDIN for executable: %w", err)
	}

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("failed to open STDOUT for executable: %w", err)
	}

	stderr, err = cmd.StderrPipe()
	if err != nil {
		return false, fmt.Errorf("failed to open STDERR for executable: %w", err)
	}

	n.logger.Debug("invoking executable notifier", "exec", n.conf.ExecFile)
	err = cmd.Start()
	if err != nil {
		return false, fmt.Errorf("unable to start executable: %w", err)
	}

	err = n.dispatchExecPayload(ctx, stdin, as...)
	if err != nil {
		n.logger.Error("failed to open STDERR for executable", "fd", "stdin", "exec", n.conf.ExecFile, "err", err)
	}

	// explicitly close the input in case the executable
	// is waiting indefinetly for data
	err = stdin.Close()
	if err != nil {
		n.logger.Warn("unable to close STDIN for executable", "fd", "stdin", "exec", n.conf.ExecFile, "err", err)
	}

	err = n.logOutput(ctx, slog.LevelDebug, "stdout", stdout)
	if err != nil {
		n.logger.Warn("unable to read STDOUT from executable", "fd", "stdout", "exec", n.conf.ExecFile, "err", err)
	}

	err = n.logOutput(ctx, slog.LevelWarn, "stderr", stderr)
	if err != nil {
		n.logger.Warn("unable to read STDERR from executable", "fd", "stderr", "exec", n.conf.ExecFile, "err", err)
	}

	var status int
	err = cmd.Wait()
	if err == nil {
		status = ExitSuccess
	} else if ee, ok := err.(*exec.ExitError); ok {
		status = ee.Sys().(syscall.WaitStatus).ExitStatus()
	} else {
		status = ExitFailure
	}

	return status == ExitRetry, err
}

func (n *Notifier) logOutput(ctx context.Context, level slog.Level, fd string, r io.Reader) error {
	if !n.logger.Enabled(ctx, level) {
		return nil
	}

	message, err := io.ReadAll(r)
	n.logger.Log(ctx, level, string(message), "fd", fd, "exec", n.conf.ExecFile)

	return err
}

// Write the given alerts to the executable input.
func (n *Notifier) dispatchExecPayload(ctx context.Context, w io.Writer, as ...*types.Alert) error {
	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return err
	}

	n.logger.Debug("extracted group key", "key", groupKey)

	data := notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
	msg := &Message{
		Version:  messageVersion,
		Data:     data,
		GroupKey: groupKey.String(),
	}

	return json.NewEncoder(w).Encode(msg)
}
