package htmx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMatchers(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{`{severity="critical"}`, 1, false},
		{`severity="critical",job=~"web.*"`, 2, false},
		{``, 0, false},
	}

	for _, tt := range tests {
		matchers, err := parseMatchers(tt.input)
		if tt.hasError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, matchers, tt.expected)
		}
	}
}

func TestParseMatcher(t *testing.T) {
	tests := []struct {
		input    string
		hasError bool
	}{
		{"severity=critical", false},
		{"severity=~web.*", false},
		{"severity!=warning", false},
		{"severity!~test.*", false},
		{"invalid", true},
	}

	for _, tt := range tests {
		_, err := parseMatcher(tt.input)
		if tt.hasError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}