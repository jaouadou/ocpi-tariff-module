package ocpi221

const (
	ReferenceGoVersion = "1.22"
	ReferenceTestStack = "testing + testify/require + go-cmp/cmp + rapid"
)

const (
	TariffElementSelectionPolicy  = "first matching element per TariffDimension (order dependent)"
	RestrictionsCombinationPolicy = "restrictions are logical AND"
)

const (
	StartDateBoundaryPolicy   = "start_date inclusive"
	EndDateBoundaryPolicy     = "end_date exclusive"
	MinKWhBoundaryPolicy      = "min_kwh inclusive"
	MaxKWhBoundaryPolicy      = "max_kwh exclusive"
	MinDurationBoundaryPolicy = "min_duration inclusive"
	MaxDurationBoundaryPolicy = "max_duration exclusive"
	MinCurrentBoundaryPolicy  = "min_current >="
	MaxCurrentBoundaryPolicy  = "max_current <"
	MinPowerBoundaryPolicy    = "min_power >="
	MaxPowerBoundaryPolicy    = "max_power <"
	StartTimeBoundaryPolicy   = "start_time inclusive"
	EndTimeBoundaryPolicy     = "end_time exclusive"
)

const TimezoneDSTPolicy = "use Go time normalization (ambiguous -> standard-time offset; nonexistent -> shift forward)"
