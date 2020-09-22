package gotime

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeInterval describes intervals of time. ContainsTime will tell you if a golang time is contained
// within the interval.
type TimeInterval struct {
	Times       []timeRange       `yaml:"times"`
	Weekdays    []weekdayRange    `yaml:"weekdays"`
	DaysOfMonth []dayOfMonthRange `yaml:"days_of_month"`
	Months      []monthRange      `yaml:"months"`
	Years       []yearRange       `yaml:"years"`
}

/* TimeRange represents a range of minutes within a 1440 minute day, exclusive of the end minute. A day consists of 1440 minutes.
   For example, 5:00PM to end of the day would begin at 1020 and end at 1440. */
type timeRange struct {
	startMinute int
	endMinute   int
}

// inclusiveRange is used to hold the beginning and end values of many time interval components
type inclusiveRange struct {
	begin int
	end   int
}

// A weekdayRange is an inclusive range between [0, 6] where 0 = Sunday
type weekdayRange struct {
	inclusiveRange
}

// A dayOfMonthRange is an inclusive range that may have negative beginning/end values that represent distance from the end of the month beginning at -1
type dayOfMonthRange struct {
	inclusiveRange
}

// A monthRange is an inclusive range between [1, 12] where 1 = January
type monthRange struct {
	inclusiveRange
}

// A year range is a positive inclusive range
type yearRange struct {
	inclusiveRange
}

type yamlTimeRange struct {
	StartTime string `yaml:"start_time"`
	EndTime   string `yaml:"end_time"`
}

// A range with a beginning and end that can be represented as strings
type stringableRange interface {
	setBegin(int)
	setEnd(int)
	// Try to map a member of the range into an integer.
	memberFromString(string) (int, error)
}

func (ir *inclusiveRange) setBegin(n int) {
	ir.begin = n
}

func (ir *inclusiveRange) setEnd(n int) {
	ir.end = n
}

func (ir *inclusiveRange) memberFromString(in string) (out int, err error) {
	out, err = strconv.Atoi(in)
	if err != nil {
		return -1, err
	}
	return out, nil
}

func (r *weekdayRange) memberFromString(in string) (out int, err error) {
	out, ok := daysOfWeek[in]
	if !ok {
		return -1, fmt.Errorf("%s is not a valid weekday", in)
	}
	return out, nil
}

func (r *monthRange) memberFromString(in string) (out int, err error) {
	out, ok := months[in]
	if !ok {
		return -1, fmt.Errorf("%s is not a valid weekday", in)
	}
	return out, nil
}

var daysOfWeek = map[string]int{
	"sunday":    0,
	"monday":    1,
	"tuesday":   2,
	"wednesday": 3,
	"thursday":  4,
	"friday":    5,
	"saturday":  6,
}

var months = map[string]int{
	"january":   1,
	"february":  2,
	"march":     3,
	"april":     4,
	"may":       5,
	"june":      6,
	"july":      7,
	"august":    8,
	"september": 9,
	"october":   10,
	"november":  11,
	"december":  12,
}

func (r *weekdayRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.begin > r.end {
		return errors.New("Start day cannot be before end day")
	}
	if r.begin < 0 || r.begin > 6 {
		return fmt.Errorf("%s is not a valid day of the week: out of range", str)
	}
	if r.end < 0 || r.end > 6 {
		return fmt.Errorf("%s is not a valid day of the week: out of range", str)
	}
	return err
}

func (r *dayOfMonthRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.begin == 0 || r.begin < -31 || r.begin > 31 {
		return fmt.Errorf("%d is not a valid day of the month: out of range", r.begin)
	}
	if r.end == 0 || r.end < -31 || r.end > 31 {
		return fmt.Errorf("%d is not a valid day of the month: out of range", r.end)
	}
	// Check beginning <= end accounting for negatives day of month indices
	trueBegin := r.begin
	trueEnd := r.end
	if r.begin < 0 {
		trueBegin = 30 + r.begin
	}
	if r.end < 0 {
		trueEnd = 30 + r.end
	}
	if trueBegin > trueEnd {
		return errors.New("Start day cannot be before end day")
	}
	return err
}

func (r *monthRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.begin > r.end {
		return errors.New("Start month cannot be before end month")
	}
	if r.begin < 1 || r.begin > 12 {
		return fmt.Errorf("%s is not a valid month: out of range", str)
	}
	if r.end < 1 || r.end > 12 {
		return fmt.Errorf("%s is not a valid month: out of range", str)
	}
	return err
}

func (r *yearRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.begin > r.end {
		return errors.New("Start day cannot be before end day")
	}
	return err
}

// UnmarshalYAML implements the Unmarshaller interface for timeRanges.
func (tr *timeRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var y yamlTimeRange
	if err := unmarshal(&y); err != nil {
		return err
	}
	if y.EndTime == "" || y.StartTime == "" {
		return errors.New("Both start and end times must be provided")
	}
	start, err := parseTime(y.StartTime)
	if err != nil {
		return nil
	}
	end, err := parseTime(y.EndTime)
	if err != nil {
		return err
	}
	if start < 0 {
		return errors.New("Start time out of range")
	}
	if end > 1440 {
		return errors.New("End time out of range")
	}
	if start >= end {
		return errors.New("Start time cannot be equal or greater than end time")
	}
	tr.startMinute, tr.endMinute = start, end
	return nil
}

// TimeLayout specifies the layout to be used in time.Parse() calls for time intervals
const TimeLayout = "15:04"

var validTime string = "^((([01][0-9])|(2[0-3])):[0-5][0-9])$|(^24:00$)"
var validTimeRE *regexp.Regexp = regexp.MustCompile(validTime)

// Given a time, determines the number of days in the month that time occurs in.
func daysInMonth(t time.Time) int {
	monthStart := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)
	diff := monthEnd.Sub(monthStart)
	return int(diff.Hours() / 24)
}

func clamp(n, min, max int) int {
	if n <= min {
		return min
	}
	if n >= max {
		return max
	}
	return n
}

// ContainsTime returns true if the TimeInterval contains the given time, otherwise returns false
func (tp TimeInterval) ContainsTime(t time.Time) bool {
	if tp.Times != nil {
		in := false
		for _, validMinutes := range tp.Times {
			if (t.Hour()*60+t.Minute()) >= validMinutes.startMinute && (t.Hour()*60+t.Minute()) < validMinutes.endMinute {
				in = true
				break
			}
		}
		if !in {
			return false
		}
	}
	if tp.DaysOfMonth != nil {
		in := false
		for _, validDates := range tp.DaysOfMonth {
			var begin, end int
			daysInMonth := daysInMonth(t)
			if validDates.begin < 0 {
				begin = daysInMonth + validDates.begin + 1
			} else {
				begin = validDates.begin
			}
			if validDates.end < 0 {
				end = daysInMonth + validDates.end + 1
			} else {
				end = validDates.end
			}
			// Clamp to the boundaries of the month to prevent crossing into other months
			begin = clamp(begin, -1*daysInMonth, daysInMonth)
			end = clamp(end, -1*daysInMonth, daysInMonth)
			if t.Day() >= begin && t.Day() <= end {
				in = true
				break
			}
		}
		if !in {
			return false
		}
	}
	if tp.Months != nil {
		in := false
		for _, validMonths := range tp.Months {
			if t.Month() >= time.Month(validMonths.begin) && t.Month() <= time.Month(validMonths.end) {
				in = true
				break
			}
		}
		if !in {
			return false
		}
	}
	if tp.Weekdays != nil {
		in := false
		for _, validDays := range tp.Weekdays {
			if t.Weekday() >= time.Weekday(validDays.begin) && t.Weekday() <= time.Weekday(validDays.end) {
				in = true
				break
			}
		}
		if !in {
			return false
		}
	}
	if tp.Years != nil {
		in := false
		for _, validYears := range tp.Years {
			if t.Year() >= validYears.begin && t.Year() <= validYears.end {
				in = true
				break
			}
		}
		if !in {
			return false
		}
	}
	return true
}

func parseTime(in string) (mins int, err error) {
	if !validTimeRE.MatchString(in) {
		return 0, fmt.Errorf("Couldn't parse timestamp %s, invalid format", in)
	}
	timestampComponents := strings.Split(in, ":")
	if len(timestampComponents) != 2 {
		return 0, fmt.Errorf("Invalid timestamp format: %s", in)
	}
	timeStampHours, err := strconv.Atoi(timestampComponents[0])
	if err != nil {
		return 0, err
	}
	timeStampMinutes, err := strconv.Atoi(timestampComponents[1])
	if err != nil {
		return 0, err
	}
	if timeStampHours < 0 || timeStampHours > 24 || timeStampMinutes < 0 || timeStampMinutes > 60 {
		return 0, fmt.Errorf("Timestamp %s out of range", in)
	}
	// Timestamps are stored as minutes elapsed in the day, so multiply hours by 60
	mins = timeStampHours*60 + timeStampMinutes
	return mins, nil
}

func stringableRangeFromString(in string, r stringableRange) (err error) {
	in = strings.ToLower(in)
	if strings.ContainsRune(in, ':') {
		components := strings.Split(in, ":")
		if len(components) != 2 {
			return fmt.Errorf("Coudn't parse range %s, invalid format", in)
		}
		start, err := r.memberFromString(components[0])
		if err != nil {
			return err
		}
		end, err := r.memberFromString(components[1])
		if err != nil {
			return err
		}
		r.setBegin(start)
		r.setEnd(end)
		return nil
	}
	val, err := r.memberFromString(in)
	if err != nil {
		return err
	}
	r.setBegin(val)
	r.setEnd(val)
	return nil
}
