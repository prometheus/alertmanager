// Copyright The Prometheus Authors
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

package common

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"
)

// NotifierConfig contains base options common across all notifier configurations.
type NotifierConfig struct {
	VSendResolved bool `yaml:"send_resolved" json:"send_resolved"`
}

func (nc *NotifierConfig) SendResolved() bool {
	return nc.VSendResolved
}

// Validator is the interface that wraps the Validate method for notifier
// configurations. Notifier config types implement this interface to provide
// per-type validation logic that can be called independently of YAML unmarshaling.
type Validator interface {
	Validate() error
}

// AsValidationError returns err as a *yaml.TypeError so that the YAML decoder
// records it and continues decoding instead of aborting at the first invalid
// notifier config. The decoder collects every such error across the document,
// which is what allows a configuration to report all of its invalid notifier
// configs at once. Errors that already are a *yaml.TypeError, including the
// decoder's own type and unknown field errors, are returned unchanged, and so
// is nil.
func AsValidationError(err error) error {
	if err == nil {
		return nil
	}
	if te, ok := errors.AsType[*yaml.TypeError](err); ok {
		return te
	}
	return &yaml.TypeError{Errors: []string{err.Error()}}
}

// FlattenValidationErrors turns the errors collected through AsValidationError
// back into plain errors at the top level of the configuration, so that a single
// validation error reads exactly as it did before and several read one per
// line. Errors reported by the decoder itself, which carry a "line N:" prefix,
// keep their usual *yaml.TypeError form.
func FlattenValidationErrors(err error) error {
	te, ok := errors.AsType[*yaml.TypeError](err)
	if !ok {
		return err
	}
	errs := make([]error, 0, len(te.Errors))
	for _, msg := range te.Errors {
		if strings.HasPrefix(msg, "line ") {
			return te
		}
		errs = append(errs, errors.New(msg))
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}
