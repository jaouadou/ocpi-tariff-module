package ocpi221

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpecDecisionConstants(t *testing.T) {
	require.Equal(t, "1.22", ReferenceGoVersion)
	require.Equal(t, "testing + testify/require + go-cmp/cmp + rapid", ReferenceTestStack)
	require.Equal(t, "first matching element per TariffDimension (order dependent)", TariffElementSelectionPolicy)
	require.Equal(t, "restrictions are logical AND", RestrictionsCombinationPolicy)
	require.Equal(t, "start_date inclusive", StartDateBoundaryPolicy)
	require.Equal(t, "end_date exclusive", EndDateBoundaryPolicy)
	require.Equal(t, "min_kwh inclusive", MinKWhBoundaryPolicy)
	require.Equal(t, "max_kwh exclusive", MaxKWhBoundaryPolicy)
	require.Equal(t, "min_duration inclusive", MinDurationBoundaryPolicy)
	require.Equal(t, "max_duration exclusive", MaxDurationBoundaryPolicy)
	require.Equal(t, "min_current >=", MinCurrentBoundaryPolicy)
	require.Equal(t, "max_current <", MaxCurrentBoundaryPolicy)
	require.Equal(t, "min_power >=", MinPowerBoundaryPolicy)
	require.Equal(t, "max_power <", MaxPowerBoundaryPolicy)
	require.Equal(t, "start_time inclusive", StartTimeBoundaryPolicy)
	require.Equal(t, "end_time exclusive", EndTimeBoundaryPolicy)
	require.Equal(t, "use Go time normalization (ambiguous -> standard-time offset; nonexistent -> shift forward)", TimezoneDSTPolicy)
}

func TestSpecDecisionsDoc(t *testing.T) {
	docPath := filepath.Join(repoRootFromThisFile(t), "README.md")
	content, err := os.ReadFile(docPath)
	require.NoError(t, err)

	doc := string(content)
	requiredPhrases := []string{
		"## Spec Decisions",
		"Tariff selection: first matching TariffElement per dimension (order dependent)",
		"Restrictions: logical AND",
		"`start_date` inclusive, `end_date` exclusive",
		"`min_kwh` inclusive, `max_kwh` exclusive",
		"`min_duration` inclusive, `max_duration` exclusive",
		"`min_current` >=, `max_current` <",
		"`min_power` >=, `max_power` <",
		"`start_time` inclusive, `end_time` exclusive",
		"Timezone/DST policy: Go time normalization",
	}

	for _, phrase := range requiredPhrases {
		require.Contains(t, doc, phrase)
	}
}

func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
}
