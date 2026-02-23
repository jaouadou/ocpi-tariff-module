package httpapi

import (
	"fmt"
	"time"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

type createSessionRequest struct {
	SessionID string        `json:"session_id"`
	StartUTC  string        `json:"start_utc"`
	Timezone  string        `json:"timezone"`
	Tariff    tariffRequest `json:"tariff"`
}

type createSessionResponse struct {
	SessionID string `json:"session_id"`
	StartUTC  string `json:"start_utc"`
	Timezone  string `json:"timezone"`
}

type ingestSamplesResponse struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
}

type meterSamplesRequest struct {
	Samples []meterSampleRequest `json:"samples"`
}

type meterSampleRequest struct {
	ID       string  `json:"id"`
	At       string  `json:"at"`
	TotalKWh float64 `json:"total_kwh"`
}

type powerSamplesRequest struct {
	Samples []powerSampleRequest `json:"samples"`
}

type powerSampleRequest struct {
	ID      string  `json:"id"`
	At      string  `json:"at"`
	PowerKW float64 `json:"power_kw"`
}

type currentSamplesRequest struct {
	Samples []currentSampleRequest `json:"samples"`
}

type currentSampleRequest struct {
	ID       string  `json:"id"`
	At       string  `json:"at"`
	CurrentA float64 `json:"current_a"`
}

type endSessionRequest struct {
	EndUTC string `json:"end_utc"`
}

type endSessionResponse struct {
	SessionID string `json:"session_id"`
	EndUTC    string `json:"end_utc"`
}

type periodsResponse struct {
	SessionID string                   `json:"session_id"`
	StartUTC  string                   `json:"start_utc"`
	EndUTC    string                   `json:"end_utc"`
	Periods   []chargingPeriodResponse `json:"periods"`
	Trace     *traceResponse           `json:"trace,omitempty"`
}

type chargingPeriodResponse struct {
	Start      string              `json:"start"`
	Dimensions []dimensionResponse `json:"dimensions"`
}

type dimensionResponse struct {
	Type   string  `json:"type"`
	Volume float64 `json:"volume"`
}

type traceResponse struct {
	Events []traceEventResponse `json:"events"`
}

type traceEventResponse struct {
	At        string `json:"at"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail"`
	PeriodKey string `json:"period_key"`
}

type cdrResponse struct {
	SessionID       string                   `json:"session_id"`
	StartUTC        string                   `json:"start_utc"`
	EndUTC          string                   `json:"end_utc"`
	FinalizedAt     string                   `json:"finalized_at"`
	ChargingPeriods []chargingPeriodResponse `json:"charging_periods"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type tariffRequest struct {
	Elements []tariffElementRequest `json:"elements"`
}

type tariffElementRequest struct {
	ID              string                    `json:"id"`
	PriceComponents []priceComponentRequest   `json:"price_components"`
	Restrictions    tariffRestrictionsRequest `json:"restrictions"`
}

type priceComponentRequest struct {
	Type string `json:"type"`
}

type tariffRestrictionsRequest struct {
	StartDate   *string  `json:"start_date"`
	EndDate     *string  `json:"end_date"`
	StartTime   *string  `json:"start_time"`
	EndTime     *string  `json:"end_time"`
	MinKWh      *float64 `json:"min_kwh"`
	MaxKWh      *float64 `json:"max_kwh"`
	MinCurrentA *float64 `json:"min_current_a"`
	MaxCurrentA *float64 `json:"max_current_a"`
	MinPowerKW  *float64 `json:"min_power_kw"`
	MaxPowerKW  *float64 `json:"max_power_kw"`
	DayOfWeek   []string `json:"day_of_week"`
	Reservation *string  `json:"reservation"`
}

func (r tariffRequest) toSegengineTariff() (segengine.Tariff, error) {
	elements := make([]segengine.TariffElement, 0, len(r.Elements))
	for _, element := range r.Elements {
		priceComponents := make([]segengine.PriceComponent, 0, len(element.PriceComponents))
		for _, component := range element.PriceComponents {
			dimension, err := parseTariffDimension(component.Type)
			if err != nil {
				return segengine.Tariff{}, err
			}
			priceComponents = append(priceComponents, segengine.PriceComponent{Type: dimension})
		}

		restrictions, err := element.Restrictions.toSegengineRestrictions()
		if err != nil {
			return segengine.Tariff{}, err
		}

		elements = append(elements, segengine.TariffElement{
			ID:              element.ID,
			PriceComponents: priceComponents,
			Restrictions:    restrictions,
		})
	}

	return segengine.Tariff{Elements: elements}, nil
}

func (r tariffRestrictionsRequest) toSegengineRestrictions() (segengine.TariffRestrictions, error) {
	dayOfWeek, err := parseWeekdays(r.DayOfWeek)
	if err != nil {
		return segengine.TariffRestrictions{}, err
	}

	reservation, err := parseReservation(r.Reservation)
	if err != nil {
		return segengine.TariffRestrictions{}, err
	}

	return segengine.TariffRestrictions{
		StartDate:   cloneStringPtr(r.StartDate),
		EndDate:     cloneStringPtr(r.EndDate),
		StartTime:   cloneStringPtr(r.StartTime),
		EndTime:     cloneStringPtr(r.EndTime),
		MinKWh:      cloneFloat64Ptr(r.MinKWh),
		MaxKWh:      cloneFloat64Ptr(r.MaxKWh),
		MinCurrentA: cloneFloat64Ptr(r.MinCurrentA),
		MaxCurrentA: cloneFloat64Ptr(r.MaxCurrentA),
		MinPowerKW:  cloneFloat64Ptr(r.MinPowerKW),
		MaxPowerKW:  cloneFloat64Ptr(r.MaxPowerKW),
		DayOfWeek:   dayOfWeek,
		Reservation: reservation,
	}, nil
}

func parseTariffDimension(raw string) (segengine.TariffDimensionType, error) {
	dimension := segengine.TariffDimensionType(raw)
	switch raw {
	case "ENERGY", "FLAT", "PARKING_TIME", "TIME", "POWER":
		return dimension, nil
	default:
		return "", fmt.Errorf("unsupported price component type %q", raw)
	}
}

func parseReservation(raw *string) (*segengine.ReservationRestrictionType, error) {
	if raw == nil {
		return nil, nil
	}

	reservation := segengine.ReservationRestrictionType(*raw)
	switch *raw {
	case "RESERVATION", "RESERVATION_EXPIRES":
		return &reservation, nil
	default:
		return nil, fmt.Errorf("unsupported reservation value %q", *raw)
	}
}

func parseWeekdays(raw []string) ([]time.Weekday, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	out := make([]time.Weekday, 0, len(raw))
	for _, day := range raw {
		parsed, err := parseWeekday(day)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}

	return out, nil
}

func parseWeekday(raw string) (time.Weekday, error) {
	switch raw {
	case "SUNDAY":
		return time.Sunday, nil
	case "MONDAY":
		return time.Monday, nil
	case "TUESDAY":
		return time.Tuesday, nil
	case "WEDNESDAY":
		return time.Wednesday, nil
	case "THURSDAY":
		return time.Thursday, nil
	case "FRIDAY":
		return time.Friday, nil
	case "SATURDAY":
		return time.Saturday, nil
	default:
		return time.Sunday, fmt.Errorf("unsupported day_of_week value %q", raw)
	}
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}
