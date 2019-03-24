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
	"io"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

const DefaultDateFormat = "2006-01-02 15:04:05 MST"

var (
	dateFormat *string
)

func InitFormatFlags(app *kingpin.Application) {
	dateFormat = app.Flag("date.format", "Format of date output").Default(DefaultDateFormat).String()
}

// Formatter needs to be implemented for each new output formatter.
type Formatter interface {
	SetOutput(io.Writer)
	FormatSilences([]types.Silence) error
	FormatAlerts([]*client.ExtendedAlert) error
	FormatConfig(*client.ServerStatus) error
}

// Formatters is a map of cli argument names to formatter interface object.
var Formatters = map[string]Formatter{}

func FormatDate(input time.Time) string {
	return input.Format(*dateFormat)
}
