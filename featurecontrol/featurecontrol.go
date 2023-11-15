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
	FeatureReceiverNameInMetrics = "receiver-name-in-metrics"
	FeatureClassicMatchers       = "classic-matchers"
	FeatureUTF8Matchers          = "utf8-matchers"
)

var AllowedFlags = []string{
	FeatureReceiverNameInMetrics,
	FeatureClassicMatchers,
	FeatureUTF8Matchers,
}

type Flagger interface {
	EnableReceiverNamesInMetrics() bool
	ClassicMatchers() bool
	UTF8Matchers() bool
}

type Flags struct {
	logger                       log.Logger
	enableReceiverNamesInMetrics bool
	classicMatchers              bool
	utf8Matchers                 bool
}

func (f *Flags) EnableReceiverNamesInMetrics() bool {
	return f.enableReceiverNamesInMetrics
}

func (f *Flags) ClassicMatchers() bool {
	return f.classicMatchers
}

func (f *Flags) UTF8Matchers() bool {
	return f.utf8Matchers
}

type flagOption func(flags *Flags)

func enableReceiverNameInMetrics() flagOption {
	return func(configs *Flags) {
		configs.enableReceiverNamesInMetrics = true
	}
}

func enableClassicMatchers() flagOption {
	return func(configs *Flags) {
		configs.classicMatchers = true
	}
}

func enableUTF8Matchers() flagOption {
	return func(configs *Flags) {
		configs.utf8Matchers = true
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
		case FeatureReceiverNameInMetrics:
			opts = append(opts, enableReceiverNameInMetrics())
			level.Warn(logger).Log("msg", "Experimental receiver name in metrics enabled")
		case FeatureClassicMatchers:
			opts = append(opts, enableClassicMatchers())
			level.Warn(logger).Log("msg", "Classic matchers parsing enabled")
		case FeatureUTF8Matchers:
			opts = append(opts, enableUTF8Matchers())
			level.Warn(logger).Log("msg", "UTF-8 matchers parsing enabled")
		default:
			return nil, fmt.Errorf("Unknown option '%s' for --enable-feature", feature)
		}
	}

	for _, opt := range opts {
		opt(fc)
	}

	if fc.classicMatchers && fc.utf8Matchers {
		return nil, errors.New("Both classic and UTF-8 matchers is enabled, please choose one or remove the flag for both")
	}

	return fc, nil
}

type NoopFlags struct{}

func (n NoopFlags) EnableReceiverNamesInMetrics() bool { return false }

func (n NoopFlags) ClassicMatchers() bool { return false }

func (n NoopFlags) UTF8Matchers() bool { return false }
