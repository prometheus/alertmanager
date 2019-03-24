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

type ExtendedFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["extended"] = &ExtendedFormatter{writer: os.Stdout}
}

func (formatter *ExtendedFormatter) SetOutput(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *ExtendedFormatter) FormatSilences(silences []types.Silence) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByEndAt(silences))
	fmt.Fprintln(w, "ID\tMatchers\tStarts At\tEnds At\tUpdated At\tCreated By\tComment\t")
	for _, silence := range silences {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n",
			silence.ID,
			extendedFormatMatchers(silence.Matchers),
			FormatDate(silence.StartsAt),
			FormatDate(silence.EndsAt),
			FormatDate(silence.UpdatedAt),
			silence.CreatedBy,
			silence.Comment,
		)
	}
	return w.Flush()
}

func (formatter *ExtendedFormatter) FormatAlerts(alerts []*client.ExtendedAlert) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByStartsAt(alerts))
	fmt.Fprintln(w, "Labels\tAnnotations\tStarts At\tEnds At\tGenerator URL\t")
	for _, alert := range alerts {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t\n",
			extendedFormatLabels(alert.Labels),
			extendedFormatAnnotations(alert.Annotations),
			FormatDate(alert.StartsAt),
			FormatDate(alert.EndsAt),
			alert.GeneratorURL,
		)
	}
	return w.Flush()
}

func (formatter *ExtendedFormatter) FormatConfig(status *client.ServerStatus) error {
	fmt.Fprintln(formatter.writer, status.ConfigYAML)
	fmt.Fprintln(formatter.writer, "buildUser", status.VersionInfo["buildUser"])
	fmt.Fprintln(formatter.writer, "goVersion", status.VersionInfo["goVersion"])
	fmt.Fprintln(formatter.writer, "revision", status.VersionInfo["revision"])
	fmt.Fprintln(formatter.writer, "version", status.VersionInfo["version"])
	fmt.Fprintln(formatter.writer, "branch", status.VersionInfo["branch"])
	fmt.Fprintln(formatter.writer, "buildDate", status.VersionInfo["buildDate"])
	fmt.Fprintln(formatter.writer, "uptime", status.Uptime)
	return nil
}

func extendedFormatLabels(labels client.LabelSet) string {
	output := []string{}
	for name, value := range labels {
		output = append(output, fmt.Sprintf("%s=\"%s\"", name, value))
	}
	sort.Strings(output)
	return strings.Join(output, " ")
}

func extendedFormatAnnotations(labels client.LabelSet) string {
	output := []string{}
	for name, value := range labels {
		output = append(output, fmt.Sprintf("%s=\"%s\"", name, value))
	}
	sort.Strings(output)
	return strings.Join(output, " ")
}

func extendedFormatMatchers(matchers types.Matchers) string {
	output := []string{}
	for _, matcher := range matchers {
		output = append(output, extendedFormatMatcher(*matcher))
	}
	return strings.Join(output, " ")
}

func extendedFormatMatcher(matcher types.Matcher) string {
	if matcher.IsRegex {
		return fmt.Sprintf("%s~=%s", matcher.Name, matcher.Value)
	}
	return fmt.Sprintf("%s=%s", matcher.Name, matcher.Value)
}
