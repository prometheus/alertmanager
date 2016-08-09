// Copyright 2016 Prometheus Team
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

package heartbeat

import (
	"time"

	"github.com/prometheus/common/log"
)

type Fanout map[string]Heartbeat

type Heartbeat interface {
	Tick() <-chan time.Time
	SendHeartbeat() error
}

// NewHeartbeatRunner returns a new HeartbeatRunner that runs a list of
// HeartbeatSender.
type HeartbeatRunner struct {
	senders map[string]Fanout
	done    map[string]chan struct{}

	log log.Logger
}

// NewHeartbeatSender returns a new HeartbeatSender.
func NewHeartbeatRunner(senders map[string]Fanout) *HeartbeatRunner {
	runner := &HeartbeatRunner{
		senders: senders,
		log:     log.With("component", "heartbeat_runner"),
		done:    make(map[string]chan struct{}),
	}
	return runner
}

func (r *HeartbeatRunner) Run() {
	for _, t := range r.senders {
		for k, s := range t {
			r.done[k] = make(chan struct{})
			go func(h Heartbeat, done chan struct{}) {
				var err error
				c := h.Tick()
				for {
					select {
					case <-c:
						err = h.SendHeartbeat()
						if err != nil {
							log.Error(err)
						}

					case <-done:
						return
					}
				}
			}(s, r.done[k])
		}
	}
}

func (r *HeartbeatRunner) Stop() {
	if r == nil {
		return
	}
	for _, t := range r.senders {
		for k, _ := range t {
			close(r.done[k])
		}
	}
}
