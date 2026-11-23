package netcdf

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// The seconds in the units of time CF names. These are defined: a minute
// is sixty seconds, an hour sixty minutes, a day twenty-four hours.
const (
	secondsPerMinute = 60
	secondsPerHour   = 60 * secondsPerMinute
	secondsPerDay    = 24 * secondsPerHour
)

// nanosecondsPerSecond is the nanoseconds in a second. It's defined.
const nanosecondsPerSecond = 1e9

// sinceWord stands between the unit and the origin in CF's time units:
// hours since 1900-01-01 00:00:00.
const sinceWord = " since "

// unitSeconds is the seconds in each unit CF's time units name, in the
// singular and the plural.
var unitSeconds = map[string]float64{
	"second": 1, "seconds": 1,
	"minute": secondsPerMinute, "minutes": secondsPerMinute,
	"hour": secondsPerHour, "hours": secondsPerHour,
	"day": secondsPerDay, "days": secondsPerDay,
}

// originLayouts are the ways an origin is written, as time.Parse takes
// them. A fraction of a second after the seconds is read by Parse
// itself, which is why the ERA-40 file's "00:00:0.0" is read.
var originLayouts = []string{"2006-1-2 15:4:5", "2006-1-2T15:4:5",
	"2006-1-2 15:4", "2006-1-2"}

// calendars are the calendars that are the one this package counts in.
var calendars = []string{"", "standard", "gregorian", "proleptic_gregorian"}

// clock turns a count of units since an origin into a time.
type clock struct {
	origin  time.Time
	seconds float64
}

// parseClock reads CF's time units: a unit, the word since, an origin.
// The error says the text it could not read.
func parseClock(text string) (*clock, error) {
	unit, origin, found := strings.Cut(strings.TrimSpace(text), sinceWord)
	seconds, known := unitSeconds[strings.ToLower(strings.TrimSpace(unit))]
	if !found || !known {
		return nil, fmt.Errorf("%q is not a unit of time (second, "+
			"minute, hour or day), then \"since\", then an origin",
			text)
	}
	origin = strings.TrimSpace(origin)
	for _, layout := range originLayouts {
		if at, err := time.Parse(layout, origin); err == nil {
			return &clock{origin: at, seconds: seconds}, nil
		}
	}
	return nil, fmt.Errorf("%q has an origin, %q, that is not YYYY-MM-DD "+
		"hh:mm:ss", text, origin)
}

// at is the time a count of units after the origin, in UTC. A count that
// is not a number has no time, and is the zero time. It counts in whole
// seconds and nanoseconds, so a far origin does not overflow.
func (counting *clock) at(count float64) time.Time {
	total := count * counting.seconds
	if math.IsNaN(total) || math.IsInf(total, 0) {
		return time.Time{}
	}
	whole := math.Floor(total)
	nanoseconds := (total - whole) * nanosecondsPerSecond
	return time.Unix(counting.origin.Unix()+int64(whole),
		int64(nanoseconds)).UTC()
}
