package tariffs

import (
	"strconv"
	"strings"
	"time"
)

func Matches(r TariffRestrictions, snap Snapshot) bool {
	local := snap.At.In(snapshotLocation(snap))

	if r.StartDate != nil && localDate(local) < *r.StartDate {
		return false
	}
	if r.EndDate != nil && localDate(local) >= *r.EndDate {
		return false
	}

	if !matchesTimeWindow(r.StartTime, r.EndTime, local) {
		return false
	}

	if r.MinKWh != nil && snap.EnergyKWh < *r.MinKWh {
		return false
	}
	if r.MaxKWh != nil && snap.EnergyKWh >= *r.MaxKWh {
		return false
	}

	if r.MinDuration != nil && snap.Duration < *r.MinDuration {
		return false
	}
	if r.MaxDuration != nil && snap.Duration >= *r.MaxDuration {
		return false
	}

	if r.MinCurrentA != nil && snap.CurrentA < *r.MinCurrentA {
		return false
	}
	if r.MaxCurrentA != nil && snap.CurrentA >= *r.MaxCurrentA {
		return false
	}
	if (r.MinCurrentA != nil || r.MaxCurrentA != nil) && !snap.CurrentKnown {
		return false
	}

	if r.MinPowerKW != nil && snap.PowerKW < *r.MinPowerKW {
		return false
	}
	if r.MaxPowerKW != nil && snap.PowerKW >= *r.MaxPowerKW {
		return false
	}
	if (r.MinPowerKW != nil || r.MaxPowerKW != nil) && !snap.PowerKnown {
		return false
	}

	if len(r.DayOfWeek) > 0 {
		matchedDay := false
		for _, day := range r.DayOfWeek {
			if local.Weekday() == day {
				matchedDay = true
				break
			}
		}
		if !matchedDay {
			return false
		}
	}

	if r.Reservation != nil {
		if snap.Reservation == nil || *snap.Reservation != *r.Reservation {
			return false
		}
	}

	return true
}

func snapshotLocation(snap Snapshot) *time.Location {
	if snap.Location == nil {
		return time.UTC
	}
	return snap.Location
}

func localDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func matchesTimeWindow(start, end *string, local time.Time) bool {
	if start == nil && end == nil {
		return true
	}

	localMinute := local.Hour()*60 + local.Minute()

	if start != nil && end != nil {
		startMinute, ok := parseClockMinute(*start)
		if !ok {
			return false
		}
		endMinute, ok := parseClockMinute(*end)
		if !ok {
			return false
		}

		if endMinute < startMinute {
			return localMinute >= startMinute || localMinute < endMinute
		}

		return localMinute >= startMinute && localMinute < endMinute
	}

	if start != nil {
		startMinute, ok := parseClockMinute(*start)
		if !ok {
			return false
		}
		return localMinute >= startMinute
	}

	endMinute, ok := parseClockMinute(*end)
	if !ok {
		return false
	}
	return localMinute < endMinute
}

func parseClockMinute(v string) (int, bool) {
	parts := strings.Split(v, ":")
	if len(parts) != 2 {
		return 0, false
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}

	return hour*60 + minute, true
}
