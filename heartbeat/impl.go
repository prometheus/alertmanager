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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/common/log"
)

type integration interface {
	Heartbeat
	name() string
}

const contentTypeJSON = "application/json"

func Build(confs []*config.Heartbeat) map[string]Fanout {
	res := map[string]Fanout{}

	for _, nc := range confs {
		var (
			hb  = Fanout{}
			add = func(i int, on integration) { hb[fmt.Sprintf("%s/%d", on.name(), i)] = on }
		)

		for i, c := range nc.OpsGenieConfigs {
			n := NewOpsGenie(c)
			add(i, n)
		}
		res[nc.Name] = hb
	}
	return res

}

// OpsGenie represents a OpsGenie implementation of Heartbeat
type OpsGenie struct {
	conf *config.HeartbeatOpsGenieConfig
	done chan struct{}

	log log.Logger
}

// Returns a new OpsGenie object
func NewOpsGenie(conf *config.HeartbeatOpsGenieConfig) *OpsGenie {
	return &OpsGenie{
		conf: conf,
		done: make(chan struct{}),
		log:  log.With("component", "opsgenie_heartbeat"),
	}
}

func (o *OpsGenie) name() string { return fmt.Sprintf("[%s] %s", "opsgenie", o.conf.Name) }

// Represents an OpsGenie heartbeat message
type opsGenieHeartBeatMessage struct {
	APIKey string `json:"apiKey"`
	Name   string `json:"name"`
}

func (o *OpsGenie) Interval() time.Duration {
	return time.Duration(o.conf.Interval)
}

func (o *OpsGenie) SendHeartbeat() error {
	var msg = &opsGenieHeartBeatMessage{
		APIKey: string(o.conf.APIKey),
		Name:   o.conf.Name,
	}
	apiURL := o.conf.APIHost + "v1/json/heartbeat/send"

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}
	log.Debugf("Sending heartbeat to %s: %s", o.conf.APIHost, buf.String())

	resp, err := http.Post(apiURL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v from %s", resp.StatusCode, apiURL)
	}

	return nil
}
