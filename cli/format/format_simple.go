package format

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/prometheus/alertmanager/types"
)

type SimpleFormatter struct {
	writer io.Writer
}

func init() {
	Formatters["simple"] = &SimpleFormatter{}
}

func (formatter *SimpleFormatter) Init(writer io.Writer) {
	formatter.writer = writer
}

func (formatter *SimpleFormatter) Format(silences []types.Silence) error {
	w := tabwriter.NewWriter(formatter.writer, 0, 0, 2, ' ', 0)
	sort.Sort(ByEndAt(silences))
	fmt.Fprintln(w, "ID\tMatchers\tEnds At\tCreated By\tComment\t")
	for _, silence := range silences {

		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t", silence.ID, simpleFormatMatchers(silence.Matchers), silence.EndsAt, silence.CreatedBy, silence.Comment)
		fmt.Fprintln(w, line)
	}
	w.Flush()
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
		return fmt.Sprintf("%s~=%s", matcher.Name, matcher.Value)
	}
	return fmt.Sprintf("%s=%s", matcher.Name, matcher.Value)
}
