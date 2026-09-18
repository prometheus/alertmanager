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
	"log/slog"
	"strings"
)

const (
	FeatureAlertNamesInMetrics   = "alert-names-in-metrics"
	FeatureReceiverNameInMetrics = "receiver-name-in-metrics"
	FeatureGroupKeyInMetrics     = "group-key-in-metrics"
	FeatureClassicMode           = "classic-mode"
	FeatureUTF8StrictMode        = "utf8-strict-mode"
	FeatureAutoGOMEMLIMIT        = "auto-gomemlimit"
	FeatureEventRecorder         = "event-recorder"
	FeatureMutedAlertsInNflog    = "muted-alerts-in-nflog"
)

var AllowedFlags = []string{
	FeatureAlertNamesInMetrics,
	FeatureReceiverNameInMetrics,
	FeatureGroupKeyInMetrics,
	FeatureClassicMode,
	FeatureUTF8StrictMode,
	FeatureAutoGOMEMLIMIT,
	FeatureEventRecorder,
	FeatureMutedAlertsInNflog,
}

type Flagger interface {
	EnableAlertNamesInMetrics() bool
	EnableReceiverNamesInMetrics() bool
	EnableGroupKeyInMetrics() bool
	ClassicMode() bool
	UTF8StrictMode() bool
	EnableAutoGOMEMLIMIT() bool
	EnableEventRecorder() bool
	EnableMutedAlertsInNflog() bool
	Enabled(feature string) bool
}

type Flags struct {
	enabled map[string]struct{}
}

func (f *Flags) EnableAlertNamesInMetrics() bool {
	return f.Enabled(FeatureAlertNamesInMetrics)
}

func (f *Flags) EnableReceiverNamesInMetrics() bool {
	return f.Enabled(FeatureReceiverNameInMetrics)
}

func (f *Flags) EnableGroupKeyInMetrics() bool {
	return f.Enabled(FeatureGroupKeyInMetrics)
}

func (f *Flags) ClassicMode() bool {
	return f.Enabled(FeatureClassicMode)
}

func (f *Flags) UTF8StrictMode() bool {
	return f.Enabled(FeatureUTF8StrictMode)
}

func (f *Flags) EnableAutoGOMEMLIMIT() bool {
	return f.Enabled(FeatureAutoGOMEMLIMIT)
}

func (f *Flags) EnableEventRecorder() bool {
	return f.Enabled(FeatureEventRecorder)
}

func (f *Flags) EnableMutedAlertsInNflog() bool {
	return f.Enabled(FeatureMutedAlertsInNflog)
}

func (f *Flags) Enabled(feature string) bool {
	_, ok := f.enabled[feature]
	return ok
}

func NewFlags(logger *slog.Logger, features string) (Flagger, error) {
	fc := &Flags{enabled: make(map[string]struct{})}

	if len(features) == 0 {
		return NoopFlags{}, nil
	}

	for feature := range strings.SplitSeq(features, ",") {
		switch feature {
		case FeatureAlertNamesInMetrics:
			logger.Warn("Alert names in metrics enabled")
		case FeatureReceiverNameInMetrics:
			logger.Warn("Experimental receiver name in metrics enabled")
		case FeatureGroupKeyInMetrics:
			logger.Warn("Experimental group key in metrics enabled")
		case FeatureClassicMode:
			logger.Warn("Classic mode enabled")
		case FeatureUTF8StrictMode:
			logger.Warn("UTF-8 strict mode enabled")
		case FeatureAutoGOMEMLIMIT:
			logger.Error("Deprecated: auto-gomemlimit will be removed in v0.35. Please use the new command line flag --auto-gomemlimit instead.")
		case FeatureEventRecorder:
			logger.Warn("Experimental event recorder enabled")
		case FeatureMutedAlertsInNflog:
			logger.Warn("Experimental muted alerts in the notification log enabled")
		default:
			return nil, fmt.Errorf("unknown option '%s' for --enable-feature", feature)
		}
		fc.enabled[feature] = struct{}{}
	}

	if fc.Enabled(FeatureClassicMode) && fc.Enabled(FeatureUTF8StrictMode) {
		return nil, errors.New("cannot have both classic and UTF-8 modes enabled")
	}

	return fc, nil
}

// IsEnabled reports whether a named feature is enabled.
func IsEnabled(flagger Flagger, feature string) bool {
	return flagger != nil && flagger.Enabled(feature)
}

type NoopFlags struct{}

func (n NoopFlags) EnableAlertNamesInMetrics() bool { return false }

func (n NoopFlags) EnableReceiverNamesInMetrics() bool { return false }

func (n NoopFlags) EnableGroupKeyInMetrics() bool { return false }

func (n NoopFlags) ClassicMode() bool { return false }

func (n NoopFlags) UTF8StrictMode() bool { return false }

func (n NoopFlags) EnableAutoGOMEMLIMIT() bool { return false }

func (n NoopFlags) EnableEventRecorder() bool { return false }

func (n NoopFlags) EnableMutedAlertsInNflog() bool { return false }

func (n NoopFlags) Enabled(string) bool { return false }
