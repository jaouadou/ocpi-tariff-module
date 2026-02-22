package ocpi

import (
	"time"

	"github.com/ocpi/ocpi/internal/periods"
)

type CDR struct {
	SessionID       string
	Start           time.Time
	End             time.Time
	ChargingPeriods []periods.ChargingPeriod
	FinalizedAt     time.Time
}
