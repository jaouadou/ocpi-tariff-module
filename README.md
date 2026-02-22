# OCPI Tariff Module: Segmentation Service

Incremental OCPI 2.2.1 charging-period segmentation engine.

A service that converts session telemetry and tariff restrictions into deterministic, ordered charging periods. It supports real-time Session-style projection and end-of-session sealed CDR finalization.

## Architecture

Core pipeline:

1. Ingest session window + telemetry + tariff.
2. Generate breakpoints from:
   - meter timestamps
   - calendar boundaries (`start_time`, `end_time`, `start_date`, `end_date`, day changes)
   - energy threshold crossings (interpolated)
   - power/current sample boundaries
3. Split into primitive intervals.
4. Evaluate active tariff elements at interval start (first-match-per-dimension).
5. Accumulate dimensions and merge adjacent stable intervals.

Implementation packages:

- `internal/events/` - deterministic event store, dedupe, watermark, quarantine/backpressure
- `internal/tariffs/` - TariffRestrictions matching and TariffElement selection
- `internal/boundaries/` - timezone-aware calendar boundaries
- `internal/breakpoints/` - interpolation and breakpoint helpers
- `internal/periods/` - period accumulation and trace mode
- `internal/ocpi/` - Session PUT projector and sealed CDR finalizer

Public package:

- `pkg/segengine` - stable external API surface

## Spec Decisions

- Tariff selection: first matching TariffElement per dimension (order dependent)
- Restrictions: logical AND
- Boundaries:
  - `start_date` inclusive, `end_date` exclusive
  - `min_kwh` inclusive, `max_kwh` exclusive
  - `min_duration` inclusive, `max_duration` exclusive
  - `min_current` >=, `max_current` <
  - `min_power` >=, `max_power` <
  - `start_time` inclusive, `end_time` exclusive
- Timezone/DST policy: Go time normalization

## Usage

Primary entrypoints (public):

- `segengine.Accumulate(...)`
- `segengine.AccumulateWithTrace(...)`
- `segengine.NewSessionProjector(...)`
- `segengine.NewFinalizer()`

Import:

```go
import segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
```

Typical flow:

1. Build tariff and telemetry slices.
2. Compute calendar boundaries and energy thresholds.
3. Call `segengine.Accumulate(...)`.
4. Optionally project Session updates and finalize CDR when watermark passes session end.

Trace mode:

- Pass `trace := &segengine.Trace{}` into `AccumulateWithTrace`.
- Inspect `trace.Events` for split reasons (`tariff_switch`, `charging_to_parking`, `meter_rollback`, etc.).

## Scope

Included:

- segmentation and charging-period construction
- deterministic behavior and hardening for missing telemetry, rollback, and event overflow

Excluded in current phase:

- pricing/cost computation (VAT/rounding/step-size billing)
- payment/invoicing flows

## Tests

Run from repo root:

```bash
go test ./...
go test ./... -run TestFixtures -count=1
go test ./... -run TestProperties -count=1
```

Fixture regression assets:

- inputs: `testdata/fixtures/*.json`
- expected outputs: `testdata/expected/*.json`
