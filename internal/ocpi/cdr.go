package ocpi

import (
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
)

type CDR struct {
	SessionID       string
	Start           time.Time
	End             time.Time
	ChargingPeriods []periods.ChargingPeriod
	FinalizedAt     time.Time
}
