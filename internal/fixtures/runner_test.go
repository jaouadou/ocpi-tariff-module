package fixtures

import (
	"encoding/json"

	"os"
	"path/filepath"
	"sort"

	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jaouadou/ocpi-tariff-module/internal/boundaries"
	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
	"github.com/jaouadou/ocpi-tariff-module/internal/tariffs"
	"github.com/stretchr/testify/require"
)

// Fixture represents the JSON structure for test fixtures
type Fixture struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	StartUTC       string          `json:"start_utc"`
	EndUTC         string          `json:"end_utc"`
	Timezone       string          `json:"timezone"`
	Tariff         FixtureTariff   `json:"tariff"`
	MeterSamples   []MeterSample   `json:"meter_samples"`
	PowerSamples   []PowerSample   `json:"power_samples,omitempty"`
	CurrentSamples []CurrentSample `json:"current_samples,omitempty"`
}

type FixtureTariff struct {
	Elements []FixtureTariffElement `json:"elements"`
}

type FixtureTariffElement struct {
	ID              string                  `json:"id"`
	PriceComponents []FixturePriceComponent `json:"price_components"`
	Restrictions    FixtureRestrictions     `json:"restrictions"`
}

type FixturePriceComponent struct {
	Type string `json:"type"`
}

type FixtureRestrictions struct {
	StartTime   *string  `json:"start_time,omitempty"`
	EndTime     *string  `json:"end_time,omitempty"`
	StartDate   *string  `json:"start_date,omitempty"`
	EndDate     *string  `json:"end_date,omitempty"`
	MinKWh      *float64 `json:"min_kwh,omitempty"`
	MaxKWh      *float64 `json:"max_kwh,omitempty"`
	MinCurrentA *float64 `json:"min_current_a,omitempty"`
	MaxCurrentA *float64 `json:"max_current_a,omitempty"`
	MinPowerKW  *float64 `json:"min_power_kw,omitempty"`
	MaxPowerKW  *float64 `json:"max_power_kw,omitempty"`
}

type MeterSample struct {
	At       string  `json:"at"`
	TotalKWh float64 `json:"total_kwh"`
}

type PowerSample struct {
	At      string  `json:"at"`
	PowerKW float64 `json:"power_kw"`
}

type CurrentSample struct {
	At       string  `json:"at"`
	CurrentA float64 `json:"current_a"`
}

// ExpectedOutput represents the expected JSON structure for test output
type ExpectedOutput struct {
	Periods []ExpectedPeriod `json:"periods"`
}

type ExpectedPeriod struct {
	Start      string              `json:"start"`
	Dimensions []ExpectedDimension `json:"dimensions"`
}

type ExpectedDimension struct {
	Type   string  `json:"type"`
	Volume float64 `json:"volume"`
}

func TestFixtures(t *testing.T) {
	fixturesDir := "../../testdata/fixtures"
	expectedDir := "../../testdata/expected"

	// Discover fixture files
	files, err := filepath.Glob(filepath.Join(fixturesDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no fixture files found")

	// Sort for deterministic order
	sort.Strings(files)

	for _, fixturePath := range files {
		t.Run(filepath.Base(fixturePath), func(t *testing.T) {
			// Load fixture
			data, err := os.ReadFile(fixturePath)
			require.NoError(t, err, "failed to read fixture")

			var fixture Fixture
			err = json.Unmarshal(data, &fixture)
			require.NoError(t, err, "failed to parse fixture")

			// Parse times
			startUTC, err := time.Parse(time.RFC3339, fixture.StartUTC)
			require.NoError(t, err, "failed to parse start_utc")
			endUTC, err := time.Parse(time.RFC3339, fixture.EndUTC)
			require.NoError(t, err, "failed to parse end_utc")

			// Parse timezone
			loc := time.UTC
			if fixture.Timezone != "" {
				loc, err = time.LoadLocation(fixture.Timezone)
				require.NoError(t, err, "failed to load timezone")
			}

			// Convert fixture tariff to internal tariff
			tariff := convertTariff(fixture.Tariff)

			// Convert meter samples
			meter := make([]breakpoints.MeterSample, len(fixture.MeterSamples))
			for i, ms := range fixture.MeterSamples {
				at, err := time.Parse(time.RFC3339, ms.At)
				require.NoError(t, err)
				meter[i] = breakpoints.MeterSample{At: at, TotalKWh: ms.TotalKWh}
			}

			// Convert power samples
			var power []breakpoints.PowerSample
			if len(fixture.PowerSamples) > 0 {
				power = make([]breakpoints.PowerSample, len(fixture.PowerSamples))
				for i, ps := range fixture.PowerSamples {
					at, err := time.Parse(time.RFC3339, ps.At)
					require.NoError(t, err)
					power[i] = breakpoints.PowerSample{At: at, PowerKW: ps.PowerKW}
				}
			}

			// Convert current samples
			var currentSamples []breakpoints.CurrentSample
			if len(fixture.CurrentSamples) > 0 {
				currentSamples = make([]breakpoints.CurrentSample, len(fixture.CurrentSamples))
				for i, cs := range fixture.CurrentSamples {
					at, err := time.Parse(time.RFC3339, cs.At)
					require.NoError(t, err)
					currentSamples[i] = breakpoints.CurrentSample{At: at, CurrentA: cs.CurrentA}
				}
			}

			// Compute calendar boundaries from tariff restrictions
			calendar := computeCalendarBoundaries(tariff, startUTC, endUTC, loc)

			// Compute energy thresholds from tariff restrictions
			thresholds := computeEnergyThresholds(tariff)

			// Run the segmentation pipeline
			result, err := periods.Accumulate(
				startUTC,
				endUTC,
				tariff,
				meter,
				power,
				currentSamples,
				calendar,
				thresholds,
			)
			require.NoError(t, err, "Accumulate failed")

			// Convert result to expected output format
			output := convertOutput(result)

			// Marshal output to JSON
			outputJSON, err := json.MarshalIndent(output, "", "  ")
			require.NoError(t, err)

			fixtureName := filepath.Base(fixturePath)
			expectedPath := filepath.Join(expectedDir, fixtureName)
			expectedData, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "expected output file not found: "+expectedPath)

			// Parse expected output
			var expected ExpectedOutput
			err = json.Unmarshal(expectedData, &expected)
			require.NoError(t, err, "failed to parse expected output")

			// Compare outputs
			diff := cmp.Diff(expected, output)
			if diff != "" {
				t.Logf("Got output:\n%s", outputJSON)
			}
			require.Empty(t, diff, "output mismatch")
		})
	}
}

func convertTariff(ft FixtureTariff) tariffs.Tariff {
	elements := make([]tariffs.TariffElement, len(ft.Elements))
	for i, fe := range ft.Elements {
		pcs := make([]tariffs.PriceComponent, len(fe.PriceComponents))
		for j, pc := range fe.PriceComponents {
			pcs[j] = tariffs.PriceComponent{Type: tariffs.TariffDimensionType(pc.Type)}
		}
		elements[i] = tariffs.TariffElement{
			ID:              fe.ID,
			PriceComponents: pcs,
			Restrictions:    convertRestrictions(fe.Restrictions),
		}
	}
	return tariffs.Tariff{Elements: elements}
}

func convertRestrictions(fr FixtureRestrictions) tariffs.TariffRestrictions {
	return tariffs.TariffRestrictions{
		StartTime:   fr.StartTime,
		EndTime:     fr.EndTime,
		StartDate:   fr.StartDate,
		EndDate:     fr.EndDate,
		MinKWh:      fr.MinKWh,
		MaxKWh:      fr.MaxKWh,
		MinCurrentA: fr.MinCurrentA,
		MaxCurrentA: fr.MaxCurrentA,
		MinPowerKW:  fr.MinPowerKW,
		MaxPowerKW:  fr.MaxPowerKW,
	}
}

func computeCalendarBoundaries(tariff tariffs.Tariff, startUTC, endUTC time.Time, loc *time.Location) []time.Time {
	var allBoundaries []time.Time

	// Collect all calendar restrictions from tariff elements
	for _, element := range tariff.Elements {
		r := element.Restrictions
		cal := boundaries.TariffRestrictionsCalendar{
			StartTime: r.StartTime,
			EndTime:   r.EndTime,
			StartDate: r.StartDate,
			EndDate:   r.EndDate,
		}
		boundaries := boundaries.CalendarBoundaries(startUTC, endUTC, loc, cal)
		allBoundaries = append(allBoundaries, boundaries...)
	}

	// Deduplicate
	if len(allBoundaries) == 0 {
		return nil
	}

	unique := make(map[int64]time.Time)
	for _, b := range allBoundaries {
		unique[b.UnixNano()] = b
	}

	result := make([]time.Time, 0, len(unique))
	for _, b := range unique {
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Before(result[j]) })

	return result
}

func computeEnergyThresholds(tariff tariffs.Tariff) []breakpoints.EnergyThreshold {
	var thresholds []breakpoints.EnergyThreshold

	for _, element := range tariff.Elements {
		r := element.Restrictions
		if r.MinKWh != nil {
			thresholds = append(thresholds, breakpoints.EnergyThreshold{
				Kind: "min",
				KWh:  *r.MinKWh,
			})
		}
		if r.MaxKWh != nil {
			thresholds = append(thresholds, breakpoints.EnergyThreshold{
				Kind: "max",
				KWh:  *r.MaxKWh,
			})
		}
	}

	return thresholds
}

func convertOutput(result []periods.ChargingPeriod) ExpectedOutput {
	output := ExpectedOutput{
		Periods: make([]ExpectedPeriod, len(result)),
	}

	for i, p := range result {
		dims := make([]ExpectedDimension, len(p.Dimensions))
		for j, d := range p.Dimensions {
			dims[j] = ExpectedDimension{
				Type:   string(d.Type),
				Volume: d.Volume,
			}
		}
		output.Periods[i] = ExpectedPeriod{
			Start:      p.Start.Format(time.RFC3339),
			Dimensions: dims,
		}
	}

	return output
}
