# OCPI Tariff Module: Segmentation Service

Incremental OCPI 2.2.1 charging-period segmentation engine.

A service that converts session telemetry and tariff restrictions into deterministic, ordered charging periods. It supports real-time Session-style projection and end-of-session sealed CDR finalization.

## HTTP Service

A standalone HTTP service exposing the segmentation engine over JSON HTTP. State is stored in-memory.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check, returns `ok` |
| GET | `/version` | Returns module and version info |
| POST | `/v1/sessions` | Create a new session |
| POST | `/v1/sessions/{id}/meter-samples` | Ingest meter samples |
| POST | `/v1/sessions/{id}/power-samples` | Ingest power samples |
| POST | `/v1/sessions/{id}/current-samples` | Ingest current samples |
| GET | `/v1/sessions/{id}/periods` | Query periods with optional trace |
| POST | `/v1/sessions/{id}/end` | End session |
| GET | `/v1/sessions/{id}/cdr` | Get sealed CDR |

### Build and Run

```bash
go build -o bin/segengine-api ./cmd/segengine-api
./bin/segengine-api --listen 127.0.0.1:8080
```

### curl Walkthrough

**Create session:**

```bash
curl -s -X POST http://127.0.0.1:8080/v1/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "start_utc": "2026-02-22T10:00:00Z",
    "timezone": "Europe/Paris",
    "tariff": {
      "elements": [
        {
          "id": "day",
          "price_components": [{"type": "ENERGY"}, {"type": "TIME"}],
          "restrictions": {
            "start_time": "08:00",
            "end_time": "20:00"
          }
        }
      ]
    }
  }'
```

Save the returned `session_id` for subsequent calls.

**Ingest meter samples:**

```bash
curl -s -X POST http://127.0.0.1:8080/v1/sessions/{session_id}/meter-samples \
  -H "Content-Type: application/json" \
  -d '{
    "samples": [
      {"id": "m1", "at": "2026-02-22T10:30:00Z", "total_kwh": 5.0},
      {"id": "m2", "at": "2026-02-22T11:00:00Z", "total_kwh": 10.0}
    ]
  }'
```

**Query periods with trace:**

```bash
curl -s "http://127.0.0.1:8080/v1/sessions/{session_id}/periods?trace=1"
```

**End session:**

```bash
curl -s -X POST http://127.0.0.1:8080/v1/sessions/{session_id}/end \
  -H "Content-Type: application/json" \
  -d '{"end_utc": "2026-02-22T12:00:00Z"}'
```

**Get sealed CDR:**

```bash
curl -s http://127.0.0.1:8080/v1/sessions/{session_id}/cdr
```

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
- `segengine.NewFinalizer()`

Import:

```go
import segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
```

Typical flow:

1. Build tariff and telemetry slices.
2. Compute calendar boundaries and energy thresholds.
3. Call `segengine.Accumulate(...)`.
4. Optionally project Session updates and finalize CDR after session end.

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
