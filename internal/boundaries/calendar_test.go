package boundaries

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalendarBoundaries_WrapAround(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	startUTC := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 6, 2, 6, 0, 0, 0, time.UTC)

	startTime := "22:00"
	endTime := "06:00"

	got := CalendarBoundaries(startUTC, endUTC, loc, TariffRestrictionsCalendar{
		StartTime: &startTime,
		EndTime:   &endTime,
	})

	gotRFC3339 := formatRFC3339(got)
	require.Equal(t, []string{
		"2026-06-01T20:00:00Z",
		"2026-06-02T04:00:00Z",
	}, gotRFC3339)
}

func TestCalendarBoundaries_DateBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)

	startUTC := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 6, 13, 22, 0, 0, 0, time.UTC)

	startDate := "2026-06-12"
	endDate := "2026-06-14"

	got := CalendarBoundaries(startUTC, endUTC, loc, TariffRestrictionsCalendar{
		StartDate: &startDate,
		EndDate:   &endDate,
	})

	gotRFC3339 := formatRFC3339(got)
	require.Equal(t, []string{
		"2026-06-11T22:00:00Z",
		"2026-06-13T22:00:00Z",
	}, gotRFC3339)
}

func formatRFC3339(instants []time.Time) []string {
	out := make([]string, 0, len(instants))
	for _, instant := range instants {
		out = append(out, instant.UTC().Format(time.RFC3339))
	}
	return out
}
