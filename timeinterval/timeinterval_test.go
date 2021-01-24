// Copyright 2020 Prometheus Team
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

package timeinterval

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

var timeIntervalTestCases = []struct {
	validTimeStrings   []string
	invalidTimeStrings []string
	timeInterval       TimeInterval
}{
	{
		timeInterval: TimeInterval{},
		validTimeStrings: []string{
			"02 Jan 06 15:04 MST",
			"03 Jan 07 10:04 MST",
			"04 Jan 06 09:04 MST",
		},
		invalidTimeStrings: []string{},
	},
	{
		// 9am to 5pm, monday to friday
		timeInterval: TimeInterval{
			Times:    []TimeRange{{StartMinute: 540, EndMinute: 1020}},
			Weekdays: []WeekdayRange{{InclusiveRange{Begin: 1, End: 5}}},
		},
		validTimeStrings: []string{
			"04 May 20 15:04 MST",
			"05 May 20 10:04 MST",
			"09 Jun 20 09:04 MST",
		},
		invalidTimeStrings: []string{
			"03 May 20 15:04 MST",
			"04 May 20 08:59 MST",
			"05 May 20 05:00 MST",
		},
	},
	{
		// Easter 2020
		timeInterval: TimeInterval{
			DaysOfMonth: []DayOfMonthRange{{InclusiveRange{Begin: 4, End: 6}}},
			Months:      []MonthRange{{InclusiveRange{Begin: 4, End: 4}}},
			Years:       []YearRange{{InclusiveRange{Begin: 2020, End: 2020}}},
		},
		validTimeStrings: []string{
			"04 Apr 20 15:04 MST",
			"05 Apr 20 00:00 MST",
			"06 Apr 20 23:05 MST",
		},
		invalidTimeStrings: []string{
			"03 May 18 15:04 MST",
			"03 Apr 20 23:59 MST",
			"04 Jun 20 23:59 MST",
			"06 Apr 19 23:59 MST",
			"07 Apr 20 00:00 MST",
		},
	},
	{
		// Check negative days of month, last 3 days of each month
		timeInterval: TimeInterval{
			DaysOfMonth: []DayOfMonthRange{{InclusiveRange{Begin: -3, End: -1}}},
		},
		validTimeStrings: []string{
			"31 Jan 20 15:04 MST",
			"30 Jan 20 15:04 MST",
			"29 Jan 20 15:04 MST",
			"30 Jun 20 00:00 MST",
			"29 Feb 20 23:05 MST",
		},
		invalidTimeStrings: []string{
			"03 May 18 15:04 MST",
			"27 Jan 20 15:04 MST",
			"03 Apr 20 23:59 MST",
			"04 Jun 20 23:59 MST",
			"06 Apr 19 23:59 MST",
			"07 Apr 20 00:00 MST",
			"01 Mar 20 00:00 MST",
		},
	},
	{
		// Check out of bound days are clamped to month boundaries
		timeInterval: TimeInterval{
			Months:      []MonthRange{{InclusiveRange{Begin: 6, End: 6}}},
			DaysOfMonth: []DayOfMonthRange{{InclusiveRange{Begin: -31, End: 31}}},
		},
		validTimeStrings: []string{
			"30 Jun 20 00:00 MST",
			"01 Jun 20 00:00 MST",
		},
		invalidTimeStrings: []string{
			"31 May 20 00:00 MST",
			"1 Jul 20 00:00 MST",
		},
	},
}

var timeStringTestCases = []struct {
	timeString  string
	TimeRange   TimeRange
	expectError bool
}{
	{
		timeString:  "{'start_time': '00:00', 'end_time': '24:00'}",
		TimeRange:   TimeRange{StartMinute: 0, EndMinute: 1440},
		expectError: false,
	},
	{
		timeString:  "{'start_time': '01:35', 'end_time': '17:39'}",
		TimeRange:   TimeRange{StartMinute: 95, EndMinute: 1059},
		expectError: false,
	},
	{
		timeString:  "{'start_time': '09:35', 'end_time': '09:39'}",
		TimeRange:   TimeRange{StartMinute: 575, EndMinute: 579},
		expectError: false,
	},
	{
		// Error: Begin and End times are the same
		timeString:  "{'start_time': '17:31', 'end_time': '17:31'}",
		TimeRange:   TimeRange{},
		expectError: true,
	},
	{
		// Error: End time out of range
		timeString:  "{'start_time': '12:30', 'end_time': '24:01'}",
		TimeRange:   TimeRange{},
		expectError: true,
	},
	{
		// Error: Start time greater than End time
		timeString:  "{'start_time': '09:30', 'end_time': '07:41'}",
		TimeRange:   TimeRange{},
		expectError: true,
	},
	{
		// Error: Start time out of range and greater than End time
		timeString:  "{'start_time': '24:00', 'end_time': '17:41'}",
		TimeRange:   TimeRange{},
		expectError: true,
	},
	{
		// Error: No range specified
		timeString:  "{'start_time': '14:03'}",
		TimeRange:   TimeRange{},
		expectError: true,
	},
}

var yamlUnmarshalTestCases = []struct {
	in          string
	intervals   []TimeInterval
	contains    []string
	excludes    []string
	expectError bool
	err         string
}{
	{
		// Simple business hours test
		in: `
---
- weekdays: ['monday:friday']
  times:
    - start_time: '09:00'
      end_time: '17:00'
`,
		intervals: []TimeInterval{
			{
				Weekdays: []WeekdayRange{{InclusiveRange{Begin: 1, End: 5}}},
				Times:    []TimeRange{{StartMinute: 540, EndMinute: 1020}},
			},
		},
		contains: []string{
			"08 Jul 20 09:00 MST",
			"08 Jul 20 16:59 MST",
		},
		excludes: []string{
			"08 Jul 20 05:00 MST",
			"08 Jul 20 08:59 MST",
		},
		expectError: false,
	},
	{
		// More advanced test with negative indices and ranges
		in: `
---
  # Last week, excluding Saturday, of the first quarter of the year during business hours from 2020 to 2025 and 2030-2035
- weekdays: ['monday:friday', 'sunday']
  months: ['january:march']
  days_of_month: ['-7:-1']
  years: ['2020:2025', '2030:2035']
  times:
    - start_time: '09:00'
      end_time: '17:00'
`,
		intervals: []TimeInterval{
			{
				Weekdays:    []WeekdayRange{{InclusiveRange{Begin: 1, End: 5}}, {InclusiveRange{Begin: 0, End: 0}}},
				Times:       []TimeRange{{StartMinute: 540, EndMinute: 1020}},
				Months:      []MonthRange{{InclusiveRange{1, 3}}},
				DaysOfMonth: []DayOfMonthRange{{InclusiveRange{-7, -1}}},
				Years:       []YearRange{{InclusiveRange{2020, 2025}}, {InclusiveRange{2030, 2035}}},
			},
		},
		contains: []string{
			"27 Jan 21 09:00 MST",
			"28 Jan 21 16:59 MST",
			"29 Jan 21 13:00 MST",
			"31 Mar 25 13:00 MST",
			"31 Mar 25 13:00 MST",
			"31 Jan 35 13:00 MST",
		},
		excludes: []string{
			"30 Jan 21 13:00 MST", // Saturday
			"01 Apr 21 13:00 MST", // 4th month
			"30 Jan 26 13:00 MST", // 2026
			"31 Jan 35 17:01 MST", // After 5pm
		},
		expectError: false,
	},
	{
		in: `
---
- weekdays: ['monday:friday']
  times:
    - start_time: '09:00'
      end_time: '17:00'`,
		intervals: []TimeInterval{
			{
				Weekdays: []WeekdayRange{{InclusiveRange{Begin: 1, End: 5}}},
				Times:    []TimeRange{{StartMinute: 540, EndMinute: 1020}},
			},
		},
		contains: []string{
			"01 Apr 21 13:00 GMT",
		},
	},
	{
		// Invalid start time.
		in: `
---
- times:
    - start_time: '01:99'
      end_time: '23:59'`,
		expectError: true,
		err:         "couldn't parse timestamp 01:99, invalid format",
	},
	{
		// Invalid end time.
		in: `
---
- times:
    - start_time: '00:00'
      end_time: '99:99'`,
		expectError: true,
		err:         "couldn't parse timestamp 99:99, invalid format",
	},
	{
		// Start day before end day.
		in: `
---
- weekdays: ['friday:monday']`,
		expectError: true,
		err:         "start day cannot be before end day",
	},
	{
		// Invalid weekdays.
		in: `
---
- weekdays: ['blurgsday:flurgsday']
`,
		expectError: true,
		err:         "blurgsday is not a valid weekday",
	},
	{
		// Numeric weekdays aren't allowed.
		in: `
---
- weekdays: ['1:3']
`,
		expectError: true,
		err:         "1 is not a valid weekday",
	},
	{
		// Negative numeric weekdays aren't allowed.
		in: `
---
- weekdays: ['-2:-1']
`,
		expectError: true,
		err:         "-2 is not a valid weekday",
	},
	{
		// 0 day of month.
		in: `
---
- days_of_month: ['0']
`,
		expectError: true,
		err:         "0 is not a valid day of the month: out of range",
	},
	{
		// Start day of month < 0.
		in: `
---
- days_of_month: ['-50:-20']
`,
		expectError: true,
		err:         "-50 is not a valid day of the month: out of range",
	},
	{
		// End day of month > 31.
		in: `
---
- days_of_month: ['1:50']
`,
		expectError: true,
		err:         "50 is not a valid day of the month: out of range",
	},
	{
		// Negative indices should work.
		in: `
---
- days_of_month: ['1:-1']
`,
		intervals: []TimeInterval{
			{
				DaysOfMonth: []DayOfMonthRange{{InclusiveRange{1, -1}}},
			},
		},
		expectError: false,
	},
	{
		// End day must be negative if begin day is negative.
		in: `
---
- days_of_month: ['-15:5']
`,
		expectError: true,
		err:         "end day must be negative if start day is negative",
	},
	{
		// Negative end date before positive postive start date.
		in: `
---
- days_of_month: ['10:-25']
`,
		expectError: true,
		err:         "end day -25 is always before start day 10",
	},
	{
		// Months should work regardless of case
		in: `
---
- months: ['January:december']
`,
		expectError: false,
		intervals: []TimeInterval{
			{
				Months: []MonthRange{{InclusiveRange{1, 12}}},
			},
		},
	},
	{
		// Invalid start month.
		in: `
---
- months: ['martius:june']
`,
		expectError: true,
		err:         "martius is not a valid month",
	},
	{
		// Invalid end month.
		in: `
---
- months: ['march:junius']
`,
		expectError: true,
		err:         "junius is not a valid month",
	},
	{
		// Start month after end month.
		in: `
---
- months: ['december:january']
`,
		expectError: true,
		err:         "end month january is before start month december",
	},
	{
		// Start year after end year.
		in: `
---
- years: ['2022:2020']
`,
		expectError: true,
		err:         "end year 2020 is before start year 2022",
	},
}

func TestYamlUnmarshal(t *testing.T) {
	for _, tc := range yamlUnmarshalTestCases {
		var ti []TimeInterval
		err := yaml.Unmarshal([]byte(tc.in), &ti)
		if err != nil && !tc.expectError {
			t.Errorf("Received unexpected error: %v when parsing %v", err, tc.in)
		} else if err == nil && tc.expectError {
			t.Errorf("Expected error when unmarshalling %s but didn't receive one", tc.in)
		} else if err != nil && tc.expectError {
			if err.Error() != tc.err {
				t.Errorf("Incorrect error: Want %s, got %s", tc.err, err.Error())
			}
			continue
		}
		if !reflect.DeepEqual(ti, tc.intervals) {
			t.Errorf("Error unmarshalling %s: Want %+v, got %+v", tc.in, tc.intervals, ti)
		}
		for _, ts := range tc.contains {
			_t, _ := time.Parse(time.RFC822, ts)
			isContained := false
			for _, interval := range ti {
				if interval.ContainsTime(_t) {
					isContained = true
				}
			}
			if !isContained {
				t.Errorf("Expected intervals to contain time %s", _t)
			}
		}
		for _, ts := range tc.excludes {
			_t, _ := time.Parse(time.RFC822, ts)
			isContained := false
			for _, interval := range ti {
				if interval.ContainsTime(_t) {
					isContained = true
				}
			}
			if isContained {
				t.Errorf("Expected intervals to exclude time %s", _t)
			}
		}
	}
}

func TestContainsTime(t *testing.T) {
	for _, tc := range timeIntervalTestCases {
		for _, ts := range tc.validTimeStrings {
			_t, _ := time.Parse(time.RFC822, ts)
			if !tc.timeInterval.ContainsTime(_t) {
				t.Errorf("Expected period %+v to contain %+v", tc.timeInterval, _t)
			}
		}
		for _, ts := range tc.invalidTimeStrings {
			_t, _ := time.Parse(time.RFC822, ts)
			if tc.timeInterval.ContainsTime(_t) {
				t.Errorf("Period %+v not expected to contain %+v", tc.timeInterval, _t)
			}
		}
	}
}

func TestParseTimeString(t *testing.T) {
	for _, tc := range timeStringTestCases {
		var tr TimeRange
		err := yaml.Unmarshal([]byte(tc.timeString), &tr)
		if err != nil && !tc.expectError {
			t.Errorf("Received unexpected error: %v when parsing %v", err, tc.timeString)
		} else if err == nil && tc.expectError {
			t.Errorf("Expected error for invalid string %s but didn't receive one", tc.timeString)
		} else if !reflect.DeepEqual(tr, tc.TimeRange) {
			t.Errorf("Error parsing time string %s: Want %+v, got %+v", tc.timeString, tc.TimeRange, tr)
		}
	}
}

func TestYamlMarshal(t *testing.T) {
	for _, tc := range yamlUnmarshalTestCases {
		if tc.expectError {
			continue
		}
		var ti []TimeInterval
		err := yaml.Unmarshal([]byte(tc.in), &ti)
		if err != nil {
			t.Error(err)
		}
		out, err := yaml.Marshal(&ti)
		if err != nil {
			t.Error(err)
		}
		var ti2 []TimeInterval
		yaml.Unmarshal(out, &ti2)
		if !reflect.DeepEqual(ti, ti2) {
			t.Errorf("Re-marshalling %s produced a different TimeInterval.", tc.in)
		}
	}
}

// Test JSON marshalling by marshalling a time interval
// and then unmarshalling to ensure they're identical
func TestJsonMarshal(t *testing.T) {
	for _, tc := range yamlUnmarshalTestCases {
		if tc.expectError {
			continue
		}
		var ti []TimeInterval
		err := yaml.Unmarshal([]byte(tc.in), &ti)
		if err != nil {
			t.Error(err)
		}
		out, err := json.Marshal(&ti)
		if err != nil {
			t.Error(err)
		}
		var ti2 []TimeInterval
		json.Unmarshal(out, &ti2)
		if !reflect.DeepEqual(ti, ti2) {
			t.Errorf("Re-marshalling %s produced a different TimeInterval. Used:\n%s and got:\n%v", tc.in, out, ti2)
		}
	}
}

var completeTestCases = []struct {
	in       string
	contains []string
	excludes []string
}{
	{
		in: `
---
weekdays: ['monday:wednesday', 'saturday', 'sunday']
times:
  - start_time: '13:00'
    end_time: '15:00'
days_of_month: ['1', '10', '20:-1']
years: ['2020:2023']
months: ['january:march']
`,
		contains: []string{
			"10 Jan 21 13:00 GMT",
			"30 Jan 21 14:24 GMT",
		},
		excludes: []string{
			"09 Jan 21 13:00 GMT",
			"20 Jan 21 12:59 GMT",
			"02 Feb 21 13:00 GMT",
		},
	},
	{
		// Check for broken clamping (clamping begin date after end of month to the end of the month)
		in: `
---
days_of_month: ['30:31']
years: ['2020:2023']
months: ['february']
`,
		excludes: []string{
			"28 Feb 21 13:00 GMT",
		},
	},
}

// Tests the entire flow from unmarshalling to containing a time
func TestTimeIntervalComplete(t *testing.T) {
	for _, tc := range completeTestCases {
		var ti TimeInterval
		if err := yaml.Unmarshal([]byte(tc.in), &ti); err != nil {
			t.Error(err)
		}
		for _, ts := range tc.contains {
			tt, err := time.Parse(time.RFC822, ts)
			if err != nil {
				t.Error(err)
			}
			if !ti.ContainsTime(tt) {
				t.Errorf("Expected %s to contain %s", tc.in, ts)
			}
		}
		for _, ts := range tc.excludes {
			tt, err := time.Parse(time.RFC822, ts)
			if err != nil {
				t.Error(err)
			}
			if ti.ContainsTime(tt) {
				t.Errorf("Expected %s to exclude %s", tc.in, ts)
			}
		}
	}
}
