package state

import (
	"testing"
	"time"

	"github.com/ocpi/ocpi/internal/breakpoints"
	"github.com/stretchr/testify/require"
)

func TestStateMachine_ChargingToParking(t *testing.T) {
	base := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)

	samples := []breakpoints.MeterSample{
		{At: base.Add(0 * time.Minute), TotalKWh: 10.0},
		{At: base.Add(5 * time.Minute), TotalKWh: 10.5},
		{At: base.Add(10 * time.Minute), TotalKWh: 11.0},
		{At: base.Add(15 * time.Minute), TotalKWh: 11.0},
		{At: base.Add(20 * time.Minute), TotalKWh: 11.0},
	}

	require.Equal(t, Charging, ClassifyInterval(samples[0], samples[1]))
	require.Equal(t, Parking, ClassifyInterval(samples[2], samples[3]))

	got := ChargingParkingBreakpoints(samples)
	require.Equal(t, []time.Time{base.Add(10 * time.Minute)}, got)
}

func TestChargingParkingBreakpoints_AlwaysCharging(t *testing.T) {
	base := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	samples := []breakpoints.MeterSample{
		{At: base.Add(0 * time.Minute), TotalKWh: 1.0},
		{At: base.Add(5 * time.Minute), TotalKWh: 1.3},
		{At: base.Add(10 * time.Minute), TotalKWh: 1.7},
		{At: base.Add(15 * time.Minute), TotalKWh: 2.0},
	}

	got := ChargingParkingBreakpoints(samples)
	require.Empty(t, got)
}

func TestChargingParkingBreakpoints_AlwaysParking(t *testing.T) {
	base := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)

	samples := []breakpoints.MeterSample{
		{At: base.Add(0 * time.Minute), TotalKWh: 3.0},
		{At: base.Add(5 * time.Minute), TotalKWh: 3.0},
		{At: base.Add(10 * time.Minute), TotalKWh: 3.0},
		{At: base.Add(15 * time.Minute), TotalKWh: 3.0},
	}

	got := ChargingParkingBreakpoints(samples)
	require.Empty(t, got)
}
