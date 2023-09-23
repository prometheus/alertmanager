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
	"errors"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	fcReceiverNameInMetrics  = "receiver-name-in-metrics"
	fcClassicMatchersParsing = "classic-matchers-parsing"
	fcUTF8MatchersParsing    = "utf8-matchers-parsing"
)

var AllowedFlags = []string{
	fcReceiverNameInMetrics,
	fcClassicMatchersParsing,
	fcUTF8MatchersParsing,
}

type Flagger interface {
	EnableReceiverNamesInMetrics() bool
	ClassicMatchersParsing() bool
	UTF8MatchersParsing() bool
}

type Flags struct {
	logger                       log.Logger
	enableReceiverNamesInMetrics bool
	classicMatchersParsing       bool
	utf8MatchersParsing          bool
}

func (f *Flags) EnableReceiverNamesInMetrics() bool {
	return f.enableReceiverNamesInMetrics
}

func (f *Flags) ClassicMatchersParsing() bool {
	return f.classicMatchersParsing
}

func (f *Flags) UTF8MatchersParsing() bool {
	return f.utf8MatchersParsing
}

type flagOption func(flags *Flags)

func enableReceiverNameInMetrics() flagOption {
	return func(configs *Flags) {
		configs.enableReceiverNamesInMetrics = true
	}
}

func enableClassicMatchersParsing() flagOption {
	return func(configs *Flags) {
		configs.classicMatchersParsing = true
	}
}

func enableUTF8MatchersParsing() flagOption {
	return func(configs *Flags) {
		configs.utf8MatchersParsing = true
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
		case fcClassicMatchersParsing:
			opts = append(opts, enableClassicMatchersParsing())
			level.Warn(logger).Log("msg", "Classic matchers parsing enabled")
		case fcUTF8MatchersParsing:
			opts = append(opts, enableUTF8MatchersParsing())
			level.Warn(logger).Log("msg", "UTF-8 matchers parsing enabled")
		default:
			return nil, fmt.Errorf("Unknown option '%s' for --enable-feature", feature)
		}
	}

	for _, opt := range opts {
		opt(fc)
	}

	if fc.classicMatchersParsing && fc.utf8MatchersParsing {
		return nil, errors.New("Both classic and UTF-8 matchers parsing is enabled, please choose one or remove the flag for both")
	}

	return fc, nil
}

type NoopFlags struct{}

func (n NoopFlags) EnableReceiverNamesInMetrics() bool { return false }

func (n NoopFlags) ClassicMatchersParsing() bool { return false }

func (n NoopFlags) UTF8MatchersParsing() bool { return false }
