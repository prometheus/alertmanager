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

package matrix

import (
	"errors"
	"strings"

	commoncfg "github.com/prometheus/common/config"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

const (
	// MsgTypeText is the message type of an ordinary message.
	MsgTypeText = "m.text"
	// MsgTypeNotice is the message type Matrix recommends for automated messages.
	MsgTypeNotice = "m.notice"
)

// DefaultMatrixConfig defines default values for Matrix configurations.
var DefaultMatrixConfig = MatrixConfig{
	NotifierConfig: amcommoncfg.NotifierConfig{
		VSendResolved: true,
	},
	Message: `{{ template "matrix.default.message" . }}`,
	MsgType: MsgTypeText,
}

// MatrixConfig configures notifications via the Matrix Client-Server API.
type MatrixConfig struct {
	amcommoncfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// HomeserverURL is the base URL of the homeserver the access token belongs to.
	HomeserverURL *amcommoncfg.URL `yaml:"homeserver_url,omitempty" json:"homeserver_url,omitempty"`
	// AccessToken authenticates the account that sends the messages.
	AccessToken     commoncfg.Secret `yaml:"access_token,omitempty" json:"access_token,omitempty"`
	AccessTokenFile string           `yaml:"access_token_file,omitempty" json:"access_token_file,omitempty"`
	// RoomID is the ID of the room to send to, such as !deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc,
	// or !abc123:example.org for rooms created with room versions before 12.
	// The account must have joined the room.
	RoomID string `yaml:"room_id,omitempty" json:"room_id,omitempty"`
	// Message is the plain text body of the message.
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
	// HTMLMessage is an optional HTML version of the message, which clients
	// show instead of the plain text body.
	HTMLMessage string `yaml:"html_message,omitempty" json:"html_message,omitempty"`
	// MsgType is m.text or m.notice.
	MsgType string `yaml:"msgtype,omitempty" json:"msgtype,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *MatrixConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultMatrixConfig
	type plain MatrixConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

// Validate checks the MatrixConfig for correctness.
func (c *MatrixConfig) Validate() error {
	if c.HomeserverURL == nil {
		return errors.New("missing homeserver_url on matrix_config")
	}
	if c.AccessToken == "" && c.AccessTokenFile == "" {
		return errors.New("missing access_token or access_token_file on matrix_config")
	}
	if c.AccessToken != "" && c.AccessTokenFile != "" {
		return errors.New("at most one of access_token & access_token_file must be configured")
	}
	if c.RoomID == "" {
		return errors.New("missing room_id on matrix_config")
	}
	// A templated room ID is checked once it is rendered.
	if !strings.Contains(c.RoomID, "{{") && !validRoomID(c.RoomID) {
		return errors.New("room_id on matrix_config must be a room ID such as !deadbeefdeadbeefdeadbeefdeadbeefdeadbeefabc, not a room alias")
	}
	if c.MsgType != MsgTypeText && c.MsgType != MsgTypeNotice {
		return errors.New("msgtype on matrix_config must be m.text or m.notice")
	}
	return nil
}

// validRoomID reports whether id has the form of a room ID. Messages can
// only be sent to a room ID; an alias such as #room:example.org would first
// have to be resolved.
func validRoomID(id string) bool {
	return strings.HasPrefix(id, "!") && len(id) > 1
}
