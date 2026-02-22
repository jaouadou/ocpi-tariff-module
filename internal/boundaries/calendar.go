package boundaries

import (
	"sort"
	"time"
)

const (
	timeLayout = "15:04"
	dateLayout = "2006-01-02"
)

type TariffRestrictionsCalendar struct {
	StartTime  *string
	EndTime    *string
	StartDate  *string
	EndDate    *string
	DaysOfWeek []time.Weekday
}

func CalendarBoundaries(startUTC, endUTC time.Time, loc *time.Location, r TariffRestrictionsCalendar) []time.Time {
	if !startUTC.Before(endUTC) {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}

	startUTC = startUTC.UTC()
	endUTC = endUTC.UTC()

	startLocal := startUTC.In(loc)
	endLocal := endUTC.In(loc)

	candidates := make(map[int64]time.Time)

	if hour, minute, ok := parseClockHHMM(r.StartTime); ok {
		addDailyClockBoundaries(candidates, startLocal, endLocal, loc, hour, minute)
	}
	if hour, minute, ok := parseClockHHMM(r.EndTime); ok {
		addDailyClockBoundaries(candidates, startLocal, endLocal, loc, hour, minute)
	}

	if date, ok := parseDateYYYYMMDD(r.StartDate, loc); ok {
		candidates[date.UnixNano()] = date.UTC()
	}
	if date, ok := parseDateYYYYMMDD(r.EndDate, loc); ok {
		candidates[date.UnixNano()] = date.UTC()
	}

	if len(r.DaysOfWeek) > 0 {
		addDailyMidnightBoundaries(candidates, startLocal, endLocal, loc)
	}

	out := make([]time.Time, 0, len(candidates))
	for _, t := range candidates {
		if t.After(startUTC) && (t.Before(endUTC) || t.Equal(endUTC)) {
			out = append(out, t.UTC())
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})

	return out
}

func parseClockHHMM(v *string) (hour int, minute int, ok bool) {
	if v == nil {
		return 0, 0, false
	}
	t, err := time.Parse(timeLayout, *v)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func parseDateYYYYMMDD(v *string, loc *time.Location) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	d, err := time.Parse(dateLayout, *v)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), true
}

func addDailyClockBoundaries(candidates map[int64]time.Time, startLocal, endLocal time.Time, loc *time.Location, hour, minute int) {
	for day := localMidnight(startLocal).AddDate(0, 0, -1); !day.After(localMidnight(endLocal).AddDate(0, 0, 1)); day = day.AddDate(0, 0, 1) {
		boundary := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc).UTC()
		candidates[boundary.UnixNano()] = boundary
	}
}

func addDailyMidnightBoundaries(candidates map[int64]time.Time, startLocal, endLocal time.Time, loc *time.Location) {
	for day := localMidnight(startLocal); !day.After(localMidnight(endLocal).AddDate(0, 0, 1)); day = day.AddDate(0, 0, 1) {
		boundary := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).UTC()
		candidates[boundary.UnixNano()] = boundary
	}
}

func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
