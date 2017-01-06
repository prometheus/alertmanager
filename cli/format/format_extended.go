package format

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
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
		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t", silence.ID, extendedFormatMatchers(silence.Matchers), silence.StartsAt, silence.EndsAt, silence.UpdatedAt, silence.CreatedBy, silence.Comment)
		fmt.Fprintln(w, line)
	}
	w.Flush()
	return nil
}

func (formatter *ExtendedFormatter) FormatAlerts(alerts model.Alerts) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByStartsAt(alerts))
	fmt.Fprintln(w, "Labels\tAnnotations\tStarts At\tEnds At\tGenerator URL\t")
	for _, alert := range alerts {
		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t", extendedFormatLabels(alert.Labels), extendedFormatAnnotations(alert.Annotations), alert.StartsAt, alert.EndsAt, alert.GeneratorURL)
		fmt.Fprintln(w, line)
	}
	w.Flush()
	return nil
}

func (formatter *ExtendedFormatter) FormatConfig(config Config) error {
	fmt.Fprintln(formatter.writer, config.Config)
	fmt.Fprintln(formatter.writer, "buildUser", config.VersionINFO["buildUser"])
	fmt.Fprintln(formatter.writer, "goVersion", config.VersionINFO["goVersion"])
	fmt.Fprintln(formatter.writer, "revision", config.VersionINFO["revision"])
	fmt.Fprintln(formatter.writer, "version", config.VersionINFO["version"])
	fmt.Fprintln(formatter.writer, "branch", config.VersionINFO["branch"])
	fmt.Fprintln(formatter.writer, "buildDate", config.VersionINFO["buildDate"])
	fmt.Fprintln(formatter.writer, "uptime", config.Uptime)
	return nil
}

func extendedFormatLabels(labels model.LabelSet) string {
	output := []string{}
	for name, value := range labels {
		output = append(output, fmt.Sprintf("%s=%s", name, value))
	}
	sort.Strings(output)
	return strings.Join(output, " ")
}

func extendedFormatAnnotations(labels model.LabelSet) string {
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
