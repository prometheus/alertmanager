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

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
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

// FormatSilences formats the silences into a readable string.
func (formatter *ExtendedFormatter) FormatSilences(silences []models.GettableSilence) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByEndAt(silences))
	fmt.Fprintln(w, "ID\tMatchers\tStarts At\tEnds At\tUpdated At\tCreated By\tComment\t")
	for _, silence := range silences {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n",
			*silence.ID,
			extendedFormatMatchers(silence.Matchers),
			FormatDate(*silence.StartsAt),
			FormatDate(*silence.EndsAt),
			FormatDate(*silence.UpdatedAt),
			*silence.CreatedBy,
			*silence.Comment,
		)
	}
	return w.Flush()
}

// FormatAlerts formats the alerts into a readable string.
func (formatter *ExtendedFormatter) FormatAlerts(alerts []*models.GettableAlert) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByStartsAt(alerts))
	fmt.Fprintln(w, "Labels\tAnnotations\tStarts At\tEnds At\tGenerator URL\tState\t")
	for _, alert := range alerts {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t\n",
			extendedFormatLabels(alert.Labels),
			extendedFormatAnnotations(alert.Annotations),
			FormatDate(*alert.StartsAt),
			FormatDate(*alert.EndsAt),
			alert.GeneratorURL,
			*alert.Status.State,
		)
	}
	return w.Flush()
}

// FormatConfig formats the alertmanager status information into a readable string.
func (formatter *ExtendedFormatter) FormatConfig(status *models.AlertmanagerStatus) error {
	fmt.Fprintln(formatter.writer, status.Config.Original)
	fmt.Fprintln(formatter.writer, "buildUser", status.VersionInfo.BuildUser)
	fmt.Fprintln(formatter.writer, "goVersion", status.VersionInfo.GoVersion)
	fmt.Fprintln(formatter.writer, "revision", status.VersionInfo.Revision)
	fmt.Fprintln(formatter.writer, "version", status.VersionInfo.Version)
	fmt.Fprintln(formatter.writer, "branch", status.VersionInfo.Branch)
	fmt.Fprintln(formatter.writer, "buildDate", status.VersionInfo.BuildDate)
	fmt.Fprintln(formatter.writer, "uptime", status.Uptime)
	return nil
}

// FormatClusterStatus formats the cluster status with peers into a readable string.
func (formatter *ExtendedFormatter) FormatClusterStatus(status *models.ClusterStatus) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w,
		"Cluster Status:\t%s\nNode Name:\t%s\n\n",
		*status.Status,
		status.Name,
	)
	fmt.Fprintln(w, "Address\tName")
	sort.Sort(ByAddress(status.Peers))
	for _, peer := range status.Peers {
		fmt.Fprintf(
			w,
			"%s\t%s\t\n",
			*peer.Address,
			*peer.Name,
		)
	}
	return w.Flush()
}

func extendedFormatLabels(labels models.LabelSet) string {
	output := []string{}
	for name, value := range labels {
		output = append(output, fmt.Sprintf("%s=\"%s\"", name, value))
	}
	sort.Strings(output)
	return strings.Join(output, " ")
}

func extendedFormatAnnotations(labels models.LabelSet) string {
	output := []string{}
	for name, value := range labels {
		output = append(output, fmt.Sprintf("%s=\"%s\"", name, value))
	}
	sort.Strings(output)
	return strings.Join(output, " ")
}

func extendedFormatMatchers(matchers models.Matchers) string {
	lms := labels.Matchers{}
	for _, matcher := range matchers {
		lms = append(lms, labelsMatcher(*matcher))
	}
	return lms.String()
}
