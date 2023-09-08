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

package featurecontrol

import (
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	fcReceiverNameInMetrics    = "receiver-name-in-metrics"
	fcDisabledNewLabelMatchers = "disable-new-label-matchers"
)

var AllowedFlags = []string{fcReceiverNameInMetrics, fcDisabledNewLabelMatchers}

type Flagger interface {
	EnableReceiverNamesInMetrics() bool
	DisableNewLabelMatchers() bool
}

type Flags struct {
	logger                       log.Logger
	enableReceiverNamesInMetrics bool
	disableNewLabelMatchers      bool
}

func (f *Flags) EnableReceiverNamesInMetrics() bool {
	return f.enableReceiverNamesInMetrics
}

func (f *Flags) DisableNewLabelMatchers() bool {
	return f.disableNewLabelMatchers
}

type flagOption func(flags *Flags)

func enableReceiverNameInMetrics() flagOption {
	return func(configs *Flags) {
		configs.enableReceiverNamesInMetrics = true
	}
}

func disableNewLabelMatchers() flagOption {
	return func(configs *Flags) {
		configs.disableNewLabelMatchers = true
	}
}

func NewFlags(logger log.Logger, features string) (Flagger, error) {
	fc := &Flags{logger: logger}
	opts := []flagOption{}

	if len(features) == 0 {
		return NoopFlags{}, nil
	}

	for _, feature := range strings.Split(features, ",") {
		switch feature {
		case fcReceiverNameInMetrics:
			opts = append(opts, enableReceiverNameInMetrics())
			level.Warn(logger).Log("msg", "Experimental receiver name in metrics enabled")
		case fcDisabledNewLabelMatchers:
			opts = append(opts, disableNewLabelMatchers())
			level.Warn(logger).Log("msg", "Disabled new label matchers")
		default:
			return nil, fmt.Errorf("Unknown option '%s' for --enable-feature", feature)
		}
	}

	for _, opt := range opts {
		opt(fc)
	}

	return fc, nil
}

type NoopFlags struct{}

func (n NoopFlags) EnableReceiverNamesInMetrics() bool { return false }

func (n NoopFlags) DisableNewLabelMatchers() bool { return false }
