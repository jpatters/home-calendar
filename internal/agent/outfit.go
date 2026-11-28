package agent

import (
	"strconv"
	"strings"
	"time"
)

// The outfit rules mirror the display's (web/src/display/agent/scene.ts) so
// Ziggy can describe what he is drawn wearing;
// web/src/display/agent/outfits.fixture.json is checked against both.

var outfitDescriptions = map[string]string{
	"winter":       "a blue sweater, dark pants, brown boots, a red toque with a white pompom and a red scarf",
	"spring":       "a yellow shirt with little gold buttons, blue pants and green shoes",
	"summer":       "a coral t-shirt, blue shorts and yellow sneakers",
	"fall":         "a brown sweater, dark pants and brown boots",
	"newyear":      "a purple shirt and a sparkly yellow-and-purple party hat",
	"valentines":   "a pink shirt with a red heart on it",
	"stpatricks":   "a green shirt and a tall green top hat with a gold buckle",
	"easter":       "a pale green shirt, lavender pants, pink shoes and bunny ears",
	"canada":       "a white shirt with a red maple leaf and red pants",
	"thanksgiving": "a brown shirt with a white collar and a black pilgrim hat with a gold buckle",
	"halloween":    "a pumpkin costume: an orange jack-o'-lantern shirt and a little green stem on your head",
	"christmas":    "a red shirt, green pants and a Santa hat",
	"pajamas":      "light blue striped pajamas and bare feet",
}

// DescribeOutfit says what an outfit looks like, for Ziggy to talk about.
func DescribeOutfit(id string) string {
	return outfitDescriptions[id]
}

// holidayLeadDays is how many days before a holiday its outfit is worn.
const holidayLeadDays = 2

// OutfitAt is the outfit Ziggy wears at t's wall-clock time: pajamas during
// quiet hours, else a holiday outfit on or shortly before a holiday, else the
// season's.
func OutfitAt(t time.Time, quietStart, quietEnd string) string {
	if inQuietHours(t, quietStart, quietEnd) {
		return "pajamas"
	}
	for ahead := 0; ahead <= holidayLeadDays; ahead++ {
		day := time.Date(t.Year(), t.Month(), t.Day()+ahead, 12, 0, 0, 0, t.Location())
		if outfit, ok := holidayOutfit(day); ok {
			return outfit
		}
	}
	switch t.Month() {
	case time.December, time.January, time.February:
		return "winter"
	case time.March, time.April, time.May:
		return "spring"
	case time.June, time.July, time.August:
		return "summer"
	default:
		return "fall"
	}
}

func holidayOutfit(day time.Time) (string, bool) {
	y, m, d := day.Date()
	switch {
	case m == time.January && d == 1:
		return "newyear", true
	case m == time.February && d == 14:
		return "valentines", true
	case m == time.March && d == 17:
		return "stpatricks", true
	case m == time.July && d == 1:
		return "canada", true
	case m == time.October && d == 31:
		return "halloween", true
	case m == time.December && d == 25:
		return "christmas", true
	}
	if em, ed := easterSunday(y); m == em && d == ed {
		return "easter", true
	}
	if m == time.October && d == thanksgivingDay(y) {
		return "thanksgiving", true
	}
	return "", false
}

// easterSunday is Gregorian Easter via the anonymous Meeus/Jones/Butcher
// algorithm.
func easterSunday(year int) (time.Month, int) {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Month(month), day
}

// thanksgivingDay is the day of October of Canadian Thanksgiving, the second
// Monday.
func thanksgivingDay(year int) int {
	firstWeekday := int(time.Date(year, time.October, 1, 12, 0, 0, 0, time.UTC).Weekday())
	firstMonday := 1 + (8-firstWeekday)%7
	return firstMonday + 7
}

// inQuietHours reports whether t's wall-clock time falls in [start, end),
// which may wrap midnight; equal start and end means never.
func inQuietHours(t time.Time, start, end string) bool {
	s, okS := clockMinutes(start)
	e, okE := clockMinutes(end)
	if !okS || !okE || s == e {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if s < e {
		return m >= s && m < e
	}
	return m >= s || m < e
}

func clockMinutes(hhmm string) (int, bool) {
	h, m, ok := strings.Cut(hhmm, ":")
	if !ok {
		return 0, false
	}
	hours, err1 := strconv.Atoi(h)
	mins, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return hours*60 + mins, true
}
