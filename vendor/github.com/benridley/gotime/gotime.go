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
	Times       []TimeRange       `yaml:"times,omitempty"`
	Weekdays    []WeekdayRange    `yaml:"weekdays,flow,omitempty"`
	DaysOfMonth []DayOfMonthRange `yaml:"days_of_month,flow,omitempty"`
	Months      []MonthRange      `yaml:"months,flow,omitempty"`
	Years       []YearRange       `yaml:"years,flow,omitempty"`
}

/* TimeRange represents a range of minutes within a 1440 minute day, exclusive of the End minute. A day consists of 1440 minutes.
   For example, 5:00PM to End of the day would Begin at 1020 and End at 1440. */
type TimeRange struct {
	StartMinute int
	EndMinute   int
}

// InclusiveRange is used to hold the Beginning and End values of many time interval components
type InclusiveRange struct {
	Begin int
	End   int
}

// A WeekdayRange is an inclusive range between [0, 6] where 0 = Sunday
type WeekdayRange struct {
	InclusiveRange
}

// A DayOfMonthRange is an inclusive range that may have negative Beginning/End values that represent distance from the End of the month Beginning at -1
type DayOfMonthRange struct {
	InclusiveRange
}

// A MonthRange is an inclusive range between [1, 12] where 1 = January
type MonthRange struct {
	InclusiveRange
}

// A YearRange is a positive inclusive range
type YearRange struct {
	InclusiveRange
}

type yamlTimeRange struct {
	StartTime string `yaml:"start_time"`
	EndTime   string `yaml:"end_time"`
}

// A range with a Beginning and End that can be represented as strings
type stringableRange interface {
	setBegin(int)
	setEnd(int)
	// Try to map a member of the range into an integer.
	memberFromString(string) (int, error)
}

func (ir *InclusiveRange) setBegin(n int) {
	ir.Begin = n
}

func (ir *InclusiveRange) setEnd(n int) {
	ir.End = n
}

func (ir *InclusiveRange) memberFromString(in string) (out int, err error) {
	out, err = strconv.Atoi(in)
	if err != nil {
		return -1, err
	}
	return out, nil
}

func (r *WeekdayRange) memberFromString(in string) (out int, err error) {
	out, ok := daysOfWeek[in]
	if !ok {
		return -1, fmt.Errorf("%s is not a valid weekday", in)
	}
	return out, nil
}

func (r *MonthRange) memberFromString(in string) (out int, err error) {
	out, ok := months[in]
	if !ok {
		out, err = strconv.Atoi(in)
		if err != nil {
			return -1, fmt.Errorf("%s is not a valid month", in)
		}
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
var daysOfWeekInv = map[int]string{
	0: "sunday",
	1: "monday",
	2: "tuesday",
	3: "wednesday",
	4: "thursday",
	5: "friday",
	6: "saturday",
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

var monthsInv = map[int]string{
	1:  "january",
	2:  "february",
	3:  "march",
	4:  "april",
	5:  "may",
	6:  "june",
	7:  "july",
	8:  "august",
	9:  "september",
	10: "october",
	11: "november",
	12: "december",
}

// UnmarshalYAML implements the Unmarshaller interface for WeekdayRange.
func (r *WeekdayRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.Begin > r.End {
		return errors.New("Start day cannot be before End day")
	}
	if r.Begin < 0 || r.Begin > 6 {
		return fmt.Errorf("%s is not a valid day of the week: out of range", str)
	}
	if r.End < 0 || r.End > 6 {
		return fmt.Errorf("%s is not a valid day of the week: out of range", str)
	}
	return err
}

// MarshalYAML implements the yaml.Marshaler interface for WeekdayRange
func (r WeekdayRange) MarshalYAML() (interface{}, error) {
	beginStr, ok := daysOfWeekInv[r.Begin]
	if !ok {
		return nil, fmt.Errorf("Unable to convert %d into weekday string", r.Begin)
	}
	if r.Begin == r.End {
		return interface{}(beginStr), nil
	}
	endStr, ok := daysOfWeekInv[r.End]
	if !ok {
		return nil, fmt.Errorf("Unable to convert %d into weekday string", r.End)
	}
	rangeStr := fmt.Sprintf("%s:%s", beginStr, endStr)
	return interface{}(rangeStr), nil
}

// UnmarshalYAML implements the Unmarshaller interface for DayOfMonthRange.
func (r *DayOfMonthRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.Begin == 0 || r.Begin < -31 || r.Begin > 31 {
		return fmt.Errorf("%d is not a valid day of the month: out of range", r.Begin)
	}
	if r.End == 0 || r.End < -31 || r.End > 31 {
		return fmt.Errorf("%d is not a valid day of the month: out of range", r.End)
	}
	// Check Beginning <= End accounting for negatives day of month indices
	trueBegin := r.Begin
	trueEnd := r.End
	if r.Begin < 0 {
		trueBegin = 30 + r.Begin
	}
	if r.End < 0 {
		trueEnd = 30 + r.End
	}
	if trueBegin > trueEnd {
		return errors.New("Start day cannot be before End day")
	}
	return err
}

// UnmarshalYAML implements the Unmarshaller interface for MonthRange.
func (r *MonthRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.Begin > r.End {
		return errors.New("Start month cannot be before End month")
	}
	if r.Begin < 1 || r.Begin > 12 {
		return fmt.Errorf("%s is not a valid month: out of range", str)
	}
	if r.End < 1 || r.End > 12 {
		return fmt.Errorf("%s is not a valid month: out of range", str)
	}
	return err
}

// MarshalYAML implements the yaml.Marshaler interface for DayOfMonthRange
func (r MonthRange) MarshalYAML() (interface{}, error) {
	beginStr, ok := monthsInv[r.Begin]
	if !ok {
		return nil, fmt.Errorf("Unable to convert %d into month", r.Begin)
	}
	if r.Begin == r.End {
		return interface{}(beginStr), nil
	}
	endStr, ok := monthsInv[r.End]
	if !ok {
		return nil, fmt.Errorf("Unable to convert %d into month", r.End)
	}
	rangeStr := fmt.Sprintf("%s:%s", beginStr, endStr)
	return interface{}(rangeStr), nil
}

// UnmarshalYAML implements the Unmarshaller interface for YearRange.
func (r *YearRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	err := stringableRangeFromString(str, r)
	if r.Begin > r.End {
		return errors.New("Start day cannot be before End day")
	}
	return err
}

// UnmarshalYAML implements the Unmarshaller interface for TimeRanges.
func (tr *TimeRange) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var y yamlTimeRange
	if err := unmarshal(&y); err != nil {
		return err
	}
	if y.EndTime == "" || y.StartTime == "" {
		return errors.New("Both start and End times must be provided")
	}
	start, err := parseTime(y.StartTime)
	if err != nil {
		return nil
	}
	End, err := parseTime(y.EndTime)
	if err != nil {
		return err
	}
	if start < 0 {
		return errors.New("Start time out of range")
	}
	if End > 1440 {
		return errors.New("End time out of range")
	}
	if start >= End {
		return errors.New("Start time cannot be equal or greater than End time")
	}
	tr.StartMinute, tr.EndMinute = start, End
	return nil
}

//MarshalYAML implements the yaml.Marshaler interface for TimeRange
func (tr TimeRange) MarshalYAML() (out interface{}, err error) {
	startHr := tr.StartMinute / 60
	endHr := tr.EndMinute / 60
	startMin := tr.StartMinute % 60
	endMin := tr.EndMinute % 60

	startStr := fmt.Sprintf("%02d:%02d", startHr, startMin)
	endStr := fmt.Sprintf("%02d:%02d", endHr, endMin)

	yTr := yamlTimeRange{startStr, endStr}
	return interface{}(yTr), err
}

//MarshalYAML implements the yaml.Marshaler interface for InclusiveRange
func (ir InclusiveRange) MarshalYAML() (interface{}, error) {
	if ir.Begin == ir.End {
		return strconv.Itoa(ir.Begin), nil
	}
	out := fmt.Sprintf("%d:%d", ir.Begin, ir.End)
	return interface{}(out), nil
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
			if (t.Hour()*60+t.Minute()) >= validMinutes.StartMinute && (t.Hour()*60+t.Minute()) < validMinutes.EndMinute {
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
			var Begin, End int
			daysInMonth := daysInMonth(t)
			if validDates.Begin < 0 {
				Begin = daysInMonth + validDates.Begin + 1
			} else {
				Begin = validDates.Begin
			}
			if validDates.End < 0 {
				End = daysInMonth + validDates.End + 1
			} else {
				End = validDates.End
			}
			// Clamp to the boundaries of the month to prevent crossing into other months
			Begin = clamp(Begin, -1*daysInMonth, daysInMonth)
			End = clamp(End, -1*daysInMonth, daysInMonth)
			if t.Day() >= Begin && t.Day() <= End {
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
			if t.Month() >= time.Month(validMonths.Begin) && t.Month() <= time.Month(validMonths.End) {
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
			if t.Weekday() >= time.Weekday(validDays.Begin) && t.Weekday() <= time.Weekday(validDays.End) {
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
			if t.Year() >= validYears.Begin && t.Year() <= validYears.End {
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

// Converts a string of the form "HH:MM" into a TimeRange
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

// Converts a range that can be represented as strings (e.g. monday:wednesday) into an equivalent integer-represented range
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
		End, err := r.memberFromString(components[1])
		if err != nil {
			return err
		}
		r.setBegin(start)
		r.setEnd(End)
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
