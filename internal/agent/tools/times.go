package tools

import (
	"fmt"
	"time"
)

const (
	localMinuteLayout = "2006-01-02T15:04"
	dateLayout        = "2006-01-02"
)

// timeFormatHelp is appended to the description of every time argument.
const timeFormatHelp = `Local time as "YYYY-MM-DDTHH:MM", a date as "YYYY-MM-DD", or RFC3339.`

// parseTime reads a tool time argument. RFC3339 values keep their instant
// and are expressed in loc; "YYYY-MM-DDTHH:MM" and "YYYY-MM-DD" are wall
// times in loc. dateOnly reports the "YYYY-MM-DD" form (local midnight).
func parseTime(s string, loc *time.Location) (t time.Time, dateOnly bool, err error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(loc), false, nil
	}
	if t, err := time.ParseInLocation(localMinuteLayout, s, loc); err == nil {
		return t, false, nil
	}
	if t, err := time.ParseInLocation(dateLayout, s, loc); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("can't read the time %q: use %s", s, timeFormatHelp)
}

// parseEnd reads the end of a range. A date-only end means the whole of that
// day, so it becomes the following midnight.
func parseEnd(s string, loc *time.Location) (time.Time, error) {
	t, dateOnly, err := parseTime(s, loc)
	if err != nil {
		return time.Time{}, err
	}
	if dateOnly {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}
