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

package test

import (
	"testing"
	"testing/synctest"
)

// SyncTestRun runs fn as a named subtest with fake time provided by
// testing/synctest. It is a convenience wrapper around t.Run and synctest.Test.
func SyncTestRun(name string, t *testing.T, fn func(t *testing.T)) {
	t.Run(name, func(t *testing.T) {
		synctest.Test(t, fn)
	})
}
