// Copyright 2018 Prometheus Team
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

package format

import (
	"encoding/json"
	"io"
	"os"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

type JSONFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["json"] = &JSONFormatter{writer: os.Stdout}
}

func (formatter *JSONFormatter) SetOutput(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *JSONFormatter) FormatSilences(silences []types.Silence) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(silences)
}

func (formatter *JSONFormatter) FormatAlerts(alerts []*client.ExtendedAlert) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(alerts)
}

func (formatter *JSONFormatter) FormatConfig(status *client.ServerStatus) error {
	enc := json.NewEncoder(formatter.writer)
	return enc.Encode(status)
}
