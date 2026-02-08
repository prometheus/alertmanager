// Copyright Prometheus Team
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
	"log/slog"
	"strings"
)

const (
	FeatureAlertNamesInMetrics   = "alert-names-in-metrics"
	FeatureReceiverNameInMetrics = "receiver-name-in-metrics"
	FeatureClassicMode           = "classic-mode"
	FeatureUTF8StrictMode        = "utf8-strict-mode"
	FeatureAutoGOMEMLIMIT        = "auto-gomemlimit"
	FeatureAutoGOMAXPROCS        = "auto-gomaxprocs"
	FeatureUIV2                  = "ui-v2"
)

var AllowedFlags = []string{
	FeatureAlertNamesInMetrics,
	FeatureReceiverNameInMetrics,
	FeatureClassicMode,
	FeatureUTF8StrictMode,
	FeatureAutoGOMEMLIMIT,
	FeatureAutoGOMAXPROCS,
	FeatureUIV2,
}

type Flagger interface {
	EnableAlertNamesInMetrics() bool
	EnableReceiverNamesInMetrics() bool
	ClassicMode() bool
	UTF8StrictMode() bool
	EnableAutoGOMEMLIMIT() bool
	EnableAutoGOMAXPROCS() bool
	EnableUIV2() bool
}

type Flags struct {
	logger                       *slog.Logger
	enableAlertNamesInMetrics    bool
	enableReceiverNamesInMetrics bool
	classicMode                  bool
	utf8StrictMode               bool
	enableAutoGOMEMLIMIT         bool
	enableAutoGOMAXPROCS         bool
	enableUIV2                   bool
}

func (f *Flags) EnableAlertNamesInMetrics() bool {
	return f.enableAlertNamesInMetrics
}

func (f *Flags) EnableReceiverNamesInMetrics() bool {
	return f.enableReceiverNamesInMetrics
}

func (f *Flags) ClassicMode() bool {
	return f.classicMode
}

func (f *Flags) UTF8StrictMode() bool {
	return f.utf8StrictMode
}

func (f *Flags) EnableAutoGOMEMLIMIT() bool {
	return f.enableAutoGOMEMLIMIT
}

func (f *Flags) EnableAutoGOMAXPROCS() bool {
	return f.enableAutoGOMAXPROCS
}

func (f *Flags) EnableUIV2() bool {
	return f.enableUIV2
}

type flagOption func(flags *Flags)

func enableReceiverNameInMetrics() flagOption {
	return func(configs *Flags) {
		configs.enableReceiverNamesInMetrics = true
	}
}

func enableClassicMode() flagOption {
	return func(configs *Flags) {
		configs.classicMode = true
	}
}

func enableUTF8StrictMode() flagOption {
	return func(configs *Flags) {
		configs.utf8StrictMode = true
	}
}

func enableAutoGOMEMLIMIT() flagOption {
	return func(configs *Flags) {
		configs.enableAutoGOMEMLIMIT = true
	}
}

func enableAutoGOMAXPROCS() flagOption {
	return func(configs *Flags) {
		configs.enableAutoGOMAXPROCS = true
	}
}

func enableAlertNamesInMetrics() flagOption {
	return func(configs *Flags) {
		configs.enableAlertNamesInMetrics = true
	}
}

func enableUIV2() flagOption {
	return func(configs *Flags) {
		configs.enableUIV2 = true
	}
}

func NewFlags(logger *slog.Logger, features string) (Flagger, error) {
	fc := &Flags{logger: logger}
	opts := []flagOption{}

	if len(features) == 0 {
		return NoopFlags{}, nil
	}

	for feature := range strings.SplitSeq(features, ",") {
		switch feature {
		case FeatureAlertNamesInMetrics:
			opts = append(opts, enableAlertNamesInMetrics())
			logger.Warn("Alert names in metrics enabled")
		case FeatureReceiverNameInMetrics:
			opts = append(opts, enableReceiverNameInMetrics())
			logger.Warn("Experimental receiver name in metrics enabled")
		case FeatureClassicMode:
			opts = append(opts, enableClassicMode())
			logger.Warn("Classic mode enabled")
		case FeatureUTF8StrictMode:
			opts = append(opts, enableUTF8StrictMode())
			logger.Warn("UTF-8 strict mode enabled")
		case FeatureAutoGOMEMLIMIT:
			opts = append(opts, enableAutoGOMEMLIMIT())
			logger.Warn("Automatically set GOMEMLIMIT to match the Linux container or system memory limit.")
		case FeatureAutoGOMAXPROCS:
			opts = append(opts, enableAutoGOMAXPROCS())
			logger.Warn("Automatically set GOMAXPROCS to match Linux container CPU quota")
		case FeatureUIV2:
			opts = append(opts, enableUIV2())
			logger.Warn("Enable the new UI (UIv2) and disable the old UI (UIv1)")
		default:
			return nil, fmt.Errorf("unknown option '%s' for --enable-feature", feature)
		}
	}

	for _, opt := range opts {
		opt(fc)
	}

	if fc.classicMode && fc.utf8StrictMode {
		return nil, errors.New("cannot have both classic and UTF-8 modes enabled")
	}

	return fc, nil
}

type NoopFlags struct{}

func (n NoopFlags) EnableAlertNamesInMetrics() bool { return false }

func (n NoopFlags) EnableReceiverNamesInMetrics() bool { return false }

func (n NoopFlags) ClassicMode() bool { return false }

func (n NoopFlags) UTF8StrictMode() bool { return false }

func (n NoopFlags) EnableAutoGOMEMLIMIT() bool { return false }

func (n NoopFlags) EnableAutoGOMAXPROCS() bool { return false }

func (n NoopFlags) EnableUIV2() bool { return false }
