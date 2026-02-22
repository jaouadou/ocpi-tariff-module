package tariffs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTariffSelector_FirstMatchPerDimension(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Amsterdam")
	snap := Snapshot{
		At:       time.Date(2026, 3, 10, 10, 30, 0, 0, time.UTC),
		Location: loc,
	}

	tariff := Tariff{
		Elements: []TariffElement{
			{
				ID: "first-energy",
				PriceComponents: []PriceComponent{
					{Type: TariffDimensionTypeEnergy},
				},
			},
			{
				ID: "first-time",
				PriceComponents: []PriceComponent{
					{Type: TariffDimensionTypeTime},
					{Type: TariffDimensionTypeEnergy},
				},
			},
			{
				ID: "later-time",
				PriceComponents: []PriceComponent{
					{Type: TariffDimensionTypeTime},
				},
			},
		},
	}

	got := SelectActiveElements(tariff, snap)

	require.Len(t, got, 2)
	require.Equal(t, "first-energy", got[TariffDimensionTypeEnergy].ID)
	require.Equal(t, "first-time", got[TariffDimensionTypeTime].ID)
}

func TestMatches_RestrictionFields(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Amsterdam")
	reservationAllowed := ReservationRestrictionTypeReservation

	base := Snapshot{
		At:           time.Date(2026, 3, 10, 10, 30, 0, 0, time.UTC),
		Location:     loc,
		EnergyKWh:    10,
		Duration:     30 * time.Minute,
		CurrentA:     16,
		CurrentKnown: true,
		PowerKW:      11,
		PowerKnown:   true,
		Reservation:  &reservationAllowed,
	}

	tests := []struct {
		name string
		r    TariffRestrictions
		want bool
	}{
		{name: "start_date inclusive", r: TariffRestrictions{StartDate: strPtr("2026-03-10")}, want: true},
		{name: "start_date blocks before", r: TariffRestrictions{StartDate: strPtr("2026-03-11")}, want: false},
		{name: "end_date allows before", r: TariffRestrictions{EndDate: strPtr("2026-03-11")}, want: true},
		{name: "end_date exclusive", r: TariffRestrictions{EndDate: strPtr("2026-03-10")}, want: false},
		{name: "start_time inclusive", r: TariffRestrictions{StartTime: strPtr("11:30")}, want: true},
		{name: "start_time blocks earlier", r: TariffRestrictions{StartTime: strPtr("11:31")}, want: false},
		{name: "end_time exclusive", r: TariffRestrictions{EndTime: strPtr("11:30")}, want: false},
		{name: "end_time allows earlier", r: TariffRestrictions{EndTime: strPtr("11:31")}, want: true},
		{name: "min_kwh inclusive", r: TariffRestrictions{MinKWh: floatPtr(10)}, want: true},
		{name: "min_kwh blocks lower", r: TariffRestrictions{MinKWh: floatPtr(10.1)}, want: false},
		{name: "max_kwh exclusive", r: TariffRestrictions{MaxKWh: floatPtr(10)}, want: false},
		{name: "max_kwh allows lower", r: TariffRestrictions{MaxKWh: floatPtr(10.1)}, want: true},
		{name: "min_duration inclusive", r: TariffRestrictions{MinDuration: durationPtr(30 * time.Minute)}, want: true},
		{name: "max_duration exclusive", r: TariffRestrictions{MaxDuration: durationPtr(30 * time.Minute)}, want: false},
		{name: "min_current inclusive", r: TariffRestrictions{MinCurrentA: floatPtr(16)}, want: true},
		{name: "max_current exclusive", r: TariffRestrictions{MaxCurrentA: floatPtr(16)}, want: false},
		{name: "min_power inclusive", r: TariffRestrictions{MinPowerKW: floatPtr(11)}, want: true},
		{name: "max_power exclusive", r: TariffRestrictions{MaxPowerKW: floatPtr(11)}, want: false},
		{name: "day_of_week includes weekday", r: TariffRestrictions{DayOfWeek: []time.Weekday{time.Tuesday}}, want: true},
		{name: "day_of_week excludes weekday", r: TariffRestrictions{DayOfWeek: []time.Weekday{time.Wednesday}}, want: false},
		{name: "reservation matches", r: TariffRestrictions{Reservation: reservationPtr(ReservationRestrictionTypeReservation)}, want: true},
		{name: "reservation mismatches", r: TariffRestrictions{Reservation: reservationPtr(ReservationRestrictionTypeReservationExpires)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Matches(tt.r, base)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMatches_ReservationMismatchWithNilSnapshotReservation(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Amsterdam")

	r := TariffRestrictions{Reservation: reservationPtr(ReservationRestrictionTypeReservation)}
	snap := Snapshot{
		At:       time.Date(2026, 3, 10, 10, 30, 0, 0, time.UTC),
		Location: loc,
	}

	require.False(t, Matches(r, snap))
}

func TestMatches_StartEndTimeWrapsMidnight(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Amsterdam")

	r := TariffRestrictions{
		StartTime: strPtr("23:00"),
		EndTime:   strPtr("06:00"),
	}

	insideLate := Snapshot{At: time.Date(2026, 3, 10, 22, 30, 0, 0, time.UTC), Location: loc}
	insideEarly := Snapshot{At: time.Date(2026, 3, 10, 4, 30, 0, 0, time.UTC), Location: loc}
	outside := Snapshot{At: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), Location: loc}

	require.True(t, Matches(r, insideLate))
	require.True(t, Matches(r, insideEarly))
	require.False(t, Matches(r, outside))
}

func TestMatches_AllRestrictionsUseLogicalAnd(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Amsterdam")
	reservationAllowed := ReservationRestrictionTypeReservation

	r := TariffRestrictions{
		StartDate:   strPtr("2026-03-10"),
		EndDate:     strPtr("2026-03-11"),
		StartTime:   strPtr("11:00"),
		EndTime:     strPtr("13:00"),
		MinKWh:      floatPtr(5),
		MaxKWh:      floatPtr(20),
		MinDuration: durationPtr(20 * time.Minute),
		MaxDuration: durationPtr(40 * time.Minute),
		MinCurrentA: floatPtr(10),
		MaxCurrentA: floatPtr(20),
		MinPowerKW:  floatPtr(5),
		MaxPowerKW:  floatPtr(15),
		DayOfWeek:   []time.Weekday{time.Tuesday},
		Reservation: reservationPtr(ReservationRestrictionTypeReservation),
	}

	allMatch := Snapshot{
		At:           time.Date(2026, 3, 10, 11, 30, 0, 0, time.UTC),
		Location:     loc,
		EnergyKWh:    10,
		Duration:     30 * time.Minute,
		CurrentA:     16,
		CurrentKnown: true,
		PowerKW:      11,
		PowerKnown:   true,
		Reservation:  &reservationAllowed,
	}
	oneFails := allMatch
	oneFails.PowerKW = 15

	require.True(t, Matches(r, allMatch))
	require.False(t, Matches(r, oneFails))
}

func TestMatches_PowerRestrictionRequiresKnownTelemetry(t *testing.T) {
	r := TariffRestrictions{MaxPowerKW: floatPtr(22)}

	snap := Snapshot{PowerKW: 0, PowerKnown: false}
	require.False(t, Matches(r, snap))

	snap.PowerKnown = true
	require.True(t, Matches(r, snap))
}

func TestMatches_CurrentRestrictionRequiresKnownTelemetry(t *testing.T) {
	r := TariffRestrictions{MaxCurrentA: floatPtr(32)}

	snap := Snapshot{CurrentA: 0, CurrentKnown: false}
	require.False(t, Matches(r, snap))

	snap.CurrentKnown = true
	require.True(t, Matches(r, snap))
}

func strPtr(v string) *string { return &v }

func floatPtr(v float64) *float64 { return &v }

func durationPtr(v time.Duration) *time.Duration { return &v }

func reservationPtr(v ReservationRestrictionType) *ReservationRestrictionType { return &v }

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}
