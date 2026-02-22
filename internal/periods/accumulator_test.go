package periods

import (
	"testing"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
	"github.com/jaouadou/ocpi-tariff-module/internal/tariffs"
	"github.com/stretchr/testify/require"
)

func TestAccumulator_MergeAdjacent(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 10, 30, 0, 0, time.UTC)

	tariff := tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "base",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
			},
		},
	}

	meter := []breakpoints.MeterSample{
		{At: startUTC, TotalKWh: 0},
		{At: startUTC.Add(10 * time.Minute), TotalKWh: 1},
		{At: startUTC.Add(20 * time.Minute), TotalKWh: 2},
		{At: endUTC, TotalKWh: 3},
	}

	calendar := []time.Time{
		startUTC.Add(5 * time.Minute),
		startUTC.Add(15 * time.Minute),
		startUTC.Add(25 * time.Minute),
	}

	got, err := Accumulate(startUTC, endUTC, tariff, meter, nil, nil, calendar, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.Equal(t, startUTC, got[0].Start)
	assertDimensionsEqual(t, []Dimension{
		{Type: DimensionTypeEnergy, Volume: 3},
		{Type: DimensionTypeTime, Volume: 0.5},
	}, got[0].Dimensions)
}

func TestAccumulator_SplitsWhenParkingStarts(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 11, 30, 0, 0, time.UTC)

	tariff := tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "base",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
					{Type: tariffs.TariffDimensionTypeParkingTime},
				},
			},
		},
	}

	meter := []breakpoints.MeterSample{
		{At: startUTC, TotalKWh: 0},
		{At: startUTC.Add(10 * time.Minute), TotalKWh: 1},
		{At: startUTC.Add(20 * time.Minute), TotalKWh: 1},
		{At: endUTC, TotalKWh: 1},
	}

	got, err := Accumulate(startUTC, endUTC, tariff, meter, nil, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)

	require.Equal(t, startUTC, got[0].Start)
	assertDimensionsEqual(t, []Dimension{
		{Type: DimensionTypeEnergy, Volume: 1},
		{Type: DimensionTypeTime, Volume: 10.0 / 60.0},
	}, got[0].Dimensions)

	require.Equal(t, startUTC.Add(10*time.Minute), got[1].Start)
	assertDimensionsEqual(t, []Dimension{
		{Type: DimensionTypeParkingTime, Volume: 20.0 / 60.0},
	}, got[1].Dimensions)
}

func TestAccumulator_SplitsOnMaxPowerRestrictionCrossing(t *testing.T) {
	startUTC := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	endUTC := time.Date(2026, 2, 22, 12, 30, 0, 0, time.UTC)
	maxPower := 5.0

	tariff := tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "low-power",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
				Restrictions: tariffs.TariffRestrictions{MaxPowerKW: &maxPower},
			},
			{
				ID: "base",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
			},
		},
	}

	meter := []breakpoints.MeterSample{
		{At: startUTC, TotalKWh: 0},
		{At: endUTC, TotalKWh: 3},
	}

	power := []breakpoints.PowerSample{
		{At: startUTC, PowerKW: 4},
		{At: startUTC.Add(15 * time.Minute), PowerKW: 6},
	}

	got, err := Accumulate(startUTC, endUTC, tariff, meter, power, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)

	require.Equal(t, startUTC, got[0].Start)
	assertDimensionsEqual(t, []Dimension{
		{Type: DimensionTypeEnergy, Volume: 1.5},
		{Type: DimensionTypeTime, Volume: 0.25},
	}, got[0].Dimensions)

	require.Equal(t, startUTC.Add(15*time.Minute), got[1].Start)
	assertDimensionsEqual(t, []Dimension{
		{Type: DimensionTypeEnergy, Volume: 1.5},
		{Type: DimensionTypeTime, Volume: 0.25},
	}, got[1].Dimensions)
}

func assertDimensionsEqual(t *testing.T, want []Dimension, got []Dimension) {
	t.Helper()

	toMap := func(in []Dimension) map[DimensionType]float64 {
		out := make(map[DimensionType]float64, len(in))
		for _, d := range in {
			out[d.Type] = d.Volume
		}
		return out
	}

	wantMap := toMap(want)
	gotMap := toMap(got)
	require.Equal(t, len(wantMap), len(gotMap))
	for typ, wantVolume := range wantMap {
		require.Contains(t, gotMap, typ)
		require.InDelta(t, wantVolume, gotMap[typ], 1e-9)
	}
}
