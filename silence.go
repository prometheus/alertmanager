// Copyright 2015 Prometheus Team
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

package main

import (
	"time"
)

type Silence struct {
	// The numeric ID of the silence.
	ID uint64

	// Name/email of the silence creator.
	CreatedBy string
	// When the silence was first created (Unix timestamp).
	CreatedAt, EndsAt time.Time

	// Additional comment about the silence.
	Comment string

	// Matchers that determine which alerts are silenced.
	Matchers Matchers

	// Timer used to trigger the deletion of the Silence after its expiry
	// time.
	expiryTimer *time.Timer
}
