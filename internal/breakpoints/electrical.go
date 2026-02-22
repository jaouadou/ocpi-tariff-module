package breakpoints

import (
	"sort"
	"time"
)

type PowerSample struct {
	At      time.Time
	PowerKW float64
}

type CurrentSample struct {
	At       time.Time
	CurrentA float64
}

func PowerRestrictionBreakpoints(samples []PowerSample, minPowerKW, maxPowerKW *float64) []time.Time {
	sorted := sortPowerSamples(samples)
	return electricalRestrictionBreakpoints(sorted, minPowerKW, maxPowerKW, func(sample powerSampleView) float64 {
		return sample.sample.PowerKW
	})
}

func CurrentRestrictionBreakpoints(samples []CurrentSample, minCurrentA, maxCurrentA *float64) []time.Time {
	sorted := sortCurrentSamples(samples)
	return electricalRestrictionBreakpoints(sorted, minCurrentA, maxCurrentA, func(sample currentSampleView) float64 {
		return sample.sample.CurrentA
	})
}

func electricalRestrictionBreakpoints[T interface{ timestamp() time.Time }](samples []T, min, max *float64, valueFn func(T) float64) []time.Time {
	if len(samples) < 2 {
		return nil
	}

	breakpoints := make(map[int64]time.Time)
	havePrevious := false
	previousMatched := false

	for _, sample := range samples {
		matched := matchesElectricalRestriction(valueFn(sample), min, max)
		if havePrevious && matched != previousMatched {
			at := sample.timestamp().UTC()
			breakpoints[at.UnixNano()] = at
		}

		previousMatched = matched
		havePrevious = true
	}

	return sortUniqueInstants(breakpoints)
}

func matchesElectricalRestriction(value float64, min, max *float64) bool {
	if min != nil && value < *min {
		return false
	}
	if max != nil && value >= *max {
		return false
	}
	return true
}

func sortPowerSamples(samples []PowerSample) []powerSampleView {
	if len(samples) == 0 {
		return nil
	}

	views := make([]powerSampleView, len(samples))
	for i := range samples {
		views[i] = powerSampleView{sample: PowerSample{At: samples[i].At.UTC(), PowerKW: samples[i].PowerKW}}
	}

	sort.Slice(views, func(i, j int) bool {
		if !views[i].sample.At.Equal(views[j].sample.At) {
			return views[i].sample.At.Before(views[j].sample.At)
		}
		return views[i].sample.PowerKW < views[j].sample.PowerKW
	})

	return views
}

func sortCurrentSamples(samples []CurrentSample) []currentSampleView {
	if len(samples) == 0 {
		return nil
	}

	views := make([]currentSampleView, len(samples))
	for i := range samples {
		views[i] = currentSampleView{sample: CurrentSample{At: samples[i].At.UTC(), CurrentA: samples[i].CurrentA}}
	}

	sort.Slice(views, func(i, j int) bool {
		if !views[i].sample.At.Equal(views[j].sample.At) {
			return views[i].sample.At.Before(views[j].sample.At)
		}
		return views[i].sample.CurrentA < views[j].sample.CurrentA
	})

	return views
}

func sortUniqueInstants(byNanos map[int64]time.Time) []time.Time {
	if len(byNanos) == 0 {
		return nil
	}

	out := make([]time.Time, 0, len(byNanos))
	for _, instant := range byNanos {
		out = append(out, instant.UTC())
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})

	return out
}

type powerSampleView struct {
	sample PowerSample
}

func (v powerSampleView) timestamp() time.Time {
	return v.sample.At
}

type currentSampleView struct {
	sample CurrentSample
}

func (v currentSampleView) timestamp() time.Time {
	return v.sample.At
}
