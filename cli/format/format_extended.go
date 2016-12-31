package format

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/prometheus/alertmanager/types"
)

type ExtendedFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["extended"] = &ExtendedFormatter{}
}

func (formatter *ExtendedFormatter) Init(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *ExtendedFormatter) Format(silences []types.Silence) error {
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
