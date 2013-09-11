// Copyright 2013 Prometheus Team
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

package config

import (
	"fmt"
	"time"

	"code.google.com/p/goprotobuf/proto"

	pb "github.com/prometheus/alertmanager/config/generated"

	"github.com/prometheus/alertmanager/manager"
)

const minimumRepeatRate = 1 * time.Minute

// Config encapsulates the configuration of an Alert Manager instance. It wraps
// the raw configuration protocol buffer to be able to add custom methods to
// it.
type Config struct {
	// The protobuf containing the actual configuration values.
	pb.AlertManagerConfig
}

// String returns an ASCII serialization of the loaded configuration protobuf.
func (c Config) String() string {
	return proto.MarshalTextString(&c.AlertManagerConfig)
}

// Validate checks an entire parsed Config for the validity of its fields.
func (c Config) Validate() error {
	ncNames := map[string]bool{}
	for _, nc := range c.NotificationConfig {
		if nc.Name == nil {
			return fmt.Errorf("Missing name in notification config: %s", proto.MarshalTextString(nc))
		}
		for _, pdc := range nc.PagerdutyConfig {
			if pdc.ServiceKey == nil {
				return fmt.Errorf("Missing service key in PagerDuty notification config: %s", proto.MarshalTextString(pdc))
			}
		}
		for _, ec := range nc.EmailConfig {
			if ec.Email == nil {
				return fmt.Errorf("Missing email address in email notification config: %s", proto.MarshalTextString(ec))
			}
		}

		if _, ok := ncNames[nc.GetName()]; ok {
			return fmt.Errorf("Notification config name not unique: %s", nc.GetName())
		}

		ncNames[nc.GetName()] = true
	}

	for _, a := range c.AggregationRule {
		for _, f := range a.Filter {
			if f.NameRe == nil {
				return fmt.Errorf("Missing name pattern (name_re) in filter definition: %s", proto.MarshalTextString(f))
			}
			if f.ValueRe == nil {
				return fmt.Errorf("Missing value pattern (value_re) in filter definition: %s", proto.MarshalTextString(f))
			}
		}

		if _, ok := ncNames[a.GetNotificationConfigName()]; !ok {
			return fmt.Errorf("No such notification config: %s", a.GetNotificationConfigName())
		}
	}

	return nil
}

func filtersFromPb(filters []*pb.Filter) manager.Filters {
	fs := make(manager.Filters, 0, len(filters))
	for _, f := range filters {
		fs = append(fs, manager.NewFilter(f.GetNameRe(), f.GetValueRe()))
	}
	return fs
}

// AggregationRules returns all the AggregationRules in a Config object.
func (c Config) AggregationRules() manager.AggregationRules {
	rules := make(manager.AggregationRules, 0, len(c.AggregationRule))
	for _, r := range c.AggregationRule {
		rate := time.Duration(r.GetRepeatRateSeconds()) * time.Second
		if rate < minimumRepeatRate {
			rate = minimumRepeatRate
		}
		rules = append(rules, &manager.AggregationRule{
			Filters:                filtersFromPb(r.Filter),
			RepeatRate:             rate,
			NotificationConfigName: r.GetNotificationConfigName(),
		})
	}
	return rules
}

// InhibitRules returns all the InhibitRules in a Config object.
func (c Config) InhibitRules() manager.InhibitRules {
	rules := make(manager.InhibitRules, 0, len(c.InhibitRule))
	for _, r := range c.InhibitRule {
		sFilters := filtersFromPb(r.SourceFilter)
		tFilters := filtersFromPb(r.TargetFilter)
		rules = append(rules, &manager.InhibitRule{
			SourceFilters: sFilters,
			TargetFilters: tFilters,
			MatchOn:       r.MatchOn,
		})
	}
	return rules
}
