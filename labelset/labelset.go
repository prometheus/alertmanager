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

package labelset

import (
	"github.com/prometheus/common/model"
)

// LabelSet wraps a model.LabelSet together with its fingerprint, so that
// consumers which already know the fingerprint (for example because they hold
// the alert it came from) do not need to hash the labels again.
// The fingerprint is set at construction time and the labels must not be
// modified afterwards.
type LabelSet struct {
	model.LabelSet

	fingerprint model.Fingerprint
}

// Fingerprint returns the fingerprint computed at construction time.
func (l LabelSet) Fingerprint() model.Fingerprint {
	return l.fingerprint
}

// New returns a LabelSet wrapping ls with fp as its fingerprint.
// The fingerprint of ls must be fp.
func New(ls model.LabelSet, fp model.Fingerprint) LabelSet {
	return LabelSet{
		LabelSet:    ls,
		fingerprint: fp,
	}
}

// FromModel returns a LabelSet wrapping ls, hashing it once to compute the
// fingerprint.
func FromModel(ls model.LabelSet) LabelSet {
	return New(ls, ls.Fingerprint())
}
