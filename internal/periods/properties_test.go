package periods

import (
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/ocpi/ocpi/internal/breakpoints"
	"github.com/ocpi/ocpi/internal/tariffs"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

const (
	propertiesRapidChecks = 120
	propertiesRapidSeed   = uint64(20260222)
	propertiesMaxSamples  = 50
	propertiesEpsilon     = 1e-9
)

type propertyCase struct {
	startUTC time.Time
	endUTC   time.Time
	meter    []breakpoints.MeterSample
	power    []breakpoints.PowerSample
	current  []breakpoints.CurrentSample
}

func TestProperties(t *testing.T) {
	setDeterministicRapidConfig(t, propertiesRapidSeed, propertiesRapidChecks)
	tariff := propertyTariff()

	rapid.Check(t, func(rt *rapid.T) {
		tc := drawPropertyCase(rt)

		periodsA, err := Accumulate(tc.startUTC, tc.endUTC, tariff, tc.meter, tc.power, tc.current, nil, nil)
		require.NoError(rt, err)

		periodsB, err := Accumulate(
			tc.startUTC,
			tc.endUTC,
			tariff,
			shuffled(tc.meter, rapid.Uint64().Draw(rt, "meter_shuffle_seed")),
			shuffled(tc.power, rapid.Uint64().Draw(rt, "power_shuffle_seed")),
			shuffled(tc.current, rapid.Uint64().Draw(rt, "current_shuffle_seed")),
			nil,
			nil,
		)
		require.NoError(rt, err)

		require.Equal(rt, periodsA, periodsB)

		normalizedMeter := normalizeMeterSamples(tc.meter)
		startTotalKWh, err := totalKWhAt(normalizedMeter, tc.startUTC)
		require.NoError(rt, err)
		endTotalKWh, err := totalKWhAt(normalizedMeter, tc.endUTC)
		require.NoError(rt, err)

		require.InDelta(rt, endTotalKWh-startTotalKWh, sumDimensionVolume(periodsA, DimensionTypeEnergy), propertiesEpsilon)
		requireStrictlyIncreasingPeriodStarts(rt, periodsA)
		requireNoNegativeDimensionVolumes(rt, periodsA)
	})
}

func setDeterministicRapidConfig(t *testing.T, seed uint64, checks int) {
	t.Helper()

	prevSeed := mustFlagValue(t, "rapid.seed")
	prevChecks := mustFlagValue(t, "rapid.checks")

	require.NoError(t, flag.Set("rapid.seed", strconv.FormatUint(seed, 10)))
	require.NoError(t, flag.Set("rapid.checks", strconv.Itoa(checks)))

	t.Cleanup(func() {
		_ = flag.Set("rapid.seed", prevSeed)
		_ = flag.Set("rapid.checks", prevChecks)
	})
}

func mustFlagValue(t *testing.T, name string) string {
	t.Helper()

	f := flag.Lookup(name)
	require.NotNil(t, f, "missing %s flag", name)
	return f.Value.String()
}

func propertyTariff() tariffs.Tariff {
	maxPowerKW := 22.0
	maxCurrentA := 32.0

	return tariffs.Tariff{
		Elements: []tariffs.TariffElement{
			{
				ID: "power-banded",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
				Restrictions: tariffs.TariffRestrictions{MaxPowerKW: &maxPowerKW},
			},
			{
				ID: "current-banded",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
				Restrictions: tariffs.TariffRestrictions{MaxCurrentA: &maxCurrentA},
			},
			{
				ID: "energy-time-fallback",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeEnergy},
					{Type: tariffs.TariffDimensionTypeTime},
				},
			},
			{
				ID: "parking-fallback",
				PriceComponents: []tariffs.PriceComponent{
					{Type: tariffs.TariffDimensionTypeParkingTime},
				},
			},
		},
	}
}

func drawPropertyCase(t *rapid.T) propertyCase {
	startUTC := time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC).Add(
		time.Duration(rapid.IntRange(0, 24*60).Draw(t, "start_offset_mins")) * time.Minute,
	)
	durationMins := rapid.IntRange(15, 12*60).Draw(t, "duration_mins")
	endUTC := startUTC.Add(time.Duration(durationMins) * time.Minute)

	meterCount := rapid.IntRange(2, propertiesMaxSamples).Draw(t, "meter_count")
	powerCount := rapid.IntRange(0, propertiesMaxSamples).Draw(t, "power_count")
	currentCount := rapid.IntRange(0, propertiesMaxSamples).Draw(t, "current_count")

	return propertyCase{
		startUTC: startUTC,
		endUTC:   endUTC,
		meter:    drawMeterSamples(t, startUTC, durationMins, meterCount),
		power:    drawPowerSamples(t, startUTC, durationMins, powerCount),
		current:  drawCurrentSamples(t, startUTC, durationMins, currentCount),
	}
}

func drawMeterSamples(t *rapid.T, startUTC time.Time, durationMins int, count int) []breakpoints.MeterSample {
	offsets := make([]int, count)
	offsets[0] = 0
	offsets[count-1] = durationMins
	for i := 1; i < count-1; i++ {
		offsets[i] = rapid.IntRange(0, durationMins).Draw(t, fmt.Sprintf("meter_offset_%d", i))
	}
	sort.Ints(offsets)

	samples := make([]breakpoints.MeterSample, count)
	total := rapid.Float64Range(0, 500).Draw(t, "meter_total_0")
	for i := range samples {
		if i > 0 {
			total += rapid.Float64Range(0, 10).Draw(t, fmt.Sprintf("meter_delta_%d", i))
		}
		samples[i] = breakpoints.MeterSample{
			At:       startUTC.Add(time.Duration(offsets[i]) * time.Minute),
			TotalKWh: total,
		}
	}

	return samples
}

func drawPowerSamples(t *rapid.T, startUTC time.Time, durationMins int, count int) []breakpoints.PowerSample {
	if count == 0 {
		return nil
	}

	samples := make([]breakpoints.PowerSample, count)
	for i := range samples {
		offset := rapid.IntRange(0, durationMins).Draw(t, fmt.Sprintf("power_offset_%d", i))
		powerKW := rapid.Float64Range(0, 50).Draw(t, fmt.Sprintf("power_kw_%d", i))
		samples[i] = breakpoints.PowerSample{
			At:      startUTC.Add(time.Duration(offset) * time.Minute),
			PowerKW: powerKW,
		}
	}

	return samples
}

func drawCurrentSamples(t *rapid.T, startUTC time.Time, durationMins int, count int) []breakpoints.CurrentSample {
	if count == 0 {
		return nil
	}

	samples := make([]breakpoints.CurrentSample, count)
	for i := range samples {
		offset := rapid.IntRange(0, durationMins).Draw(t, fmt.Sprintf("current_offset_%d", i))
		currentA := rapid.Float64Range(0, 64).Draw(t, fmt.Sprintf("current_a_%d", i))
		samples[i] = breakpoints.CurrentSample{
			At:       startUTC.Add(time.Duration(offset) * time.Minute),
			CurrentA: currentA,
		}
	}

	return samples
}

func requireStrictlyIncreasingPeriodStarts(t require.TestingT, periods []ChargingPeriod) {
	for i := 1; i < len(periods); i++ {
		require.True(t, periods[i].Start.After(periods[i-1].Start), "period starts must be strictly increasing")
	}
}

func requireNoNegativeDimensionVolumes(t require.TestingT, periods []ChargingPeriod) {
	for _, period := range periods {
		for _, dimension := range period.Dimensions {
			require.GreaterOrEqual(t, dimension.Volume, 0.0, "dimension volumes must be non-negative")
		}
	}
}

func sumDimensionVolume(periods []ChargingPeriod, dimensionType DimensionType) float64 {
	total := 0.0
	for _, period := range periods {
		for _, dimension := range period.Dimensions {
			if dimension.Type == dimensionType {
				total += dimension.Volume
			}
		}
	}
	return total
}

func shuffled[T any](in []T, seed uint64) []T {
	if len(in) == 0 {
		return nil
	}

	out := make([]T, len(in))
	copy(out, in)

	rng := rand.New(rand.NewSource(int64(seed)))
	rng.Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})

	return out
}
