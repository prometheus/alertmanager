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

	pb "github.com/prometheus/alert_manager/config/generated"

	"github.com/prometheus/alert_manager/manager"
)

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
	for _, a := range c.AggregationRule {
		for _, f := range a.Filter {
			if f.NameRe == nil {
				return fmt.Errorf("Missing name pattern (name_re) in filter definition: %s", proto.MarshalTextString(f))
			}
			if f.ValueRe == nil {
				return fmt.Errorf("Missing value pattern (value_re) in filter definition: %s", proto.MarshalTextString(f))
			}
		}
	}
	return nil
}

// Rules returns all the AggregationRules in a Config object.
func (c Config) AggregationRules() manager.AggregationRules {
	rules := make(manager.AggregationRules, 0, len(c.AggregationRule))
	for _, r := range c.AggregationRule {
		filters := make(manager.Filters, 0, len(r.Filter))
		for _, filter := range r.Filter {
			filters = append(filters, manager.NewFilter(filter.GetNameRe(), filter.GetValueRe()))
		}
		rules = append(rules, &manager.AggregationRule{
			Filters:    filters,
			RepeatRate: time.Duration(r.GetRepeatRateSeconds()) * time.Second,
		})
	}
	return rules
}
