package cli

import (
	"github.com/prometheus/alertmanager/types"
	"reflect"
	"testing"
)

// parseMatchers should be able to take a slice of strings of the form
// "foo=bar" or "foo~=bar"
// and parse them into a types.Matchers
func TestParseMatchersSimple(t *testing.T) {
	testSlice := []string{"foo=bar", "bar=baz"}
	expected := types.Matchers{
		&types.Matcher{Name: "foo", Value: "bar", IsRegex: false},
		&types.Matcher{Name: "bar", Value: "baz", IsRegex: false},
	}

	matchers, err := parseMatchers(testSlice)
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(matchers, expected) {
		t.Fatalf("Recieved: %+v Expected: %+v", expected, matchers)
	}
}

// Test the regex case of parseMatchers
func TestParseMatchersRegex(t *testing.T) {
	testSlice := []string{"foo~=bar", "bar~=baz"}
	expected := types.Matchers{
		&types.Matcher{Name: "foo", Value: "bar", IsRegex: true},
		&types.Matcher{Name: "bar", Value: "baz", IsRegex: true},
	}

	matchers, err := parseMatchers(testSlice)
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(matchers, expected) {
		t.Fatalf("Recieved: %+v Expected: %+v", expected, matchers)
	}
}

func TestParseMatcherGroups(t *testing.T) {
	testInput := types.Matchers{
		&types.Matcher{Name: "foo", Value: "bar.*", IsRegex: true},
		&types.Matcher{Name: "foo", Value: "baz", IsRegex: false},
		&types.Matcher{Name: "bar", Value: "baz", IsRegex: false},
	}

	testGroups := []types.Matchers{
		types.Matchers{
			&types.Matcher{Name: "foo", Value: "bar.*", IsRegex: true},
			&types.Matcher{Name: "bar", Value: "baz", IsRegex: false},
		},
		types.Matchers{
			&types.Matcher{Name: "foo", Value: "baz", IsRegex: false},
			&types.Matcher{Name: "bar", Value: "baz", IsRegex: false},
		},
	}

	receivedGroups := parseMatcherGroups(testInput)

	if len(receivedGroups) != len(testGroups) {
		t.Fatalf("[Size mismatch] Recieved: %+v Expected: %+v", testGroups, receivedGroups)
	}

	for groupIndex := 0; groupIndex < len(receivedGroups); groupIndex++ {
		receivedGroup := receivedGroups[groupIndex]
		testGroup := testGroups[groupIndex]
		if len(receivedGroup) != len(testGroup) {
			t.Fatalf("[Size mismatch] (index: %d) Recieved: %+v Expected: %+v", groupIndex, testGroup, receivedGroup)
		}

		for matcherIndex := 0; matcherIndex < len(receivedGroup); matcherIndex++ {
			receivedMatcher := receivedGroup[matcherIndex]
			testMatcher := testGroup[matcherIndex]
			if receivedMatcher.Name != testMatcher.Name {
				t.Fatalf("[Value mismatch name] (index: %d, %d) Recieved: %+v Expected: %+v", groupIndex, matcherIndex, receivedGroup, testGroup)
			}

			if receivedMatcher.Value != testMatcher.Value {
				t.Fatalf("[Value mismatch value] (index: %d, %d) Recieved: %+v Expected: %+v", groupIndex, matcherIndex, receivedGroup, testGroup)
			}

			if receivedMatcher.IsRegex != testMatcher.IsRegex {
				t.Fatalf("[Value mismatch regex] (index: %d, %d) Recieved: %+v Expected: %+v", groupIndex, matcherIndex, receivedGroup, testGroup)
			}
		}
	}
}
