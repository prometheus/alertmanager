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
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

type SimpleFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["simple"] = &SimpleFormatter{writer: os.Stdout}
}

func (formatter *SimpleFormatter) SetOutput(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *SimpleFormatter) FormatSilences(silences []types.Silence) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByEndAt(silences))
	fmt.Fprintln(w, "ID\tMatchers\tEnds At\tCreated By\tComment\t")
	for _, silence := range silences {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t\n",
			silence.ID,
			simpleFormatMatchers(silence.Matchers),
			FormatDate(silence.EndsAt),
			silence.CreatedBy,
			silence.Comment,
		)
	}
	return w.Flush()
}

func (formatter *SimpleFormatter) FormatAlerts(alerts []*client.ExtendedAlert) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByStartsAt(alerts))
	fmt.Fprintln(w, "Alertname\tStarts At\tSummary\t")
	for _, alert := range alerts {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t\n",
			alert.Labels["alertname"],
			FormatDate(alert.StartsAt),
			alert.Annotations["summary"],
		)
	}
	return w.Flush()
}

func (formatter *SimpleFormatter) FormatConfig(status *client.ServerStatus) error {
	fmt.Fprintln(formatter.writer, status.ConfigYAML)
	return nil
}

func simpleFormatMatchers(matchers types.Matchers) string {
	output := []string{}
	for _, matcher := range matchers {
		output = append(output, simpleFormatMatcher(*matcher))
	}
	return strings.Join(output, " ")
}

func simpleFormatMatcher(matcher types.Matcher) string {
	if matcher.IsRegex {
		return fmt.Sprintf("%s=~%s", matcher.Name, matcher.Value)
	}
	return fmt.Sprintf("%s=%s", matcher.Name, matcher.Value)
}
