# QuanTRAM Semantic Contract V1

**Date:** September 2, 2026  
**Last updated:** September 2, 2026  
**Status:** INITIAL_CANONICAL_BASELINE (cleanup closure)  
**Contract version:** `1.0`  
**JSON:** `internal/semantics/data/quantram_semantics_v1.json`  
**Parents:** [Process Model](QuanTRAM_PROCESS_MODEL_082926.md), [P-03 Implementation](QuanTRAM_P03_IMPLEMENTATION_083126.md), [P-04 Implementation](QuanTRAM_P04_IMPLEMENTATION_090226.md)

## 1. Why it exists

QuanTRAM already exposes scientific states such as HOLD, scientific SHORT, HOLDING, ON TIME, DELAYED, UP ACCELERATING, HIGH, LOW, DOMAIN EXIT, and INITIALIZING. Those meanings lived in Go source, proto, tests, and dashboard aliases.

This contract makes that vocabulary explicit, namespaced, and versioned so the Next.js viewer does not keep a second hand-written dictionary. Later Pipeline Sessions will store `semantic_contract_version` with each Scientific Frame.

The dictionary **explains** science. It does **not** drive D01, D02, D04, Emitter, Pricing, Volume, or continuity mathematics.

## 2. Authority

```text
Go executable vocabulary
        |
        v
quantram_semantics_v1.json     (source of truth)
        |
        v
internal/semantics loader
        |
        v
SemanticService (gRPC)
        |
        v
Next.js cache + SemanticHelp + /semantics
```

If a future term changes scientific meaning, increment the contract version. Do not silently rewrite historical meaning. Classification/metadata clarifications may stay on V1.0.

## 3. Canonical vs presentation

```text
canonical scientific/runtime semantic
            ->
viewer presentation alias
```

Airport/flight words (ON TIME, HOLDING, DELAYED, BOARDING) may remain on the dashboard. They are **presentation semantics**, not the stored scientific source of truth.

Every `VIEWER` term and the Arrivals IDs `PRICE_HOLDING` / `PRICE_ON_TIME` / `PRICE_DELAYED` have:

- `presentation_only = true`
- `persistence_policy = RENDER_ONLY`
- `canonical_source_ids` pointing at the domain terms they render

## 4. Persistence rule

**Persist canonical semantics. Render presentation semantics.**

Contract field:

`persistence_policy = PERSIST_CANONICAL_RENDER_PRESENTATION`

Future Pipeline Session / Scientific Frame persistence must store:

- canonical domain values (for example `color = AMBER`, Adaptive `side = HOLD`)
- stable semantic IDs where appropriate
- `semantic_contract_version`

It must **not** rely solely on transient UI labels such as Holding, On Time, or Delayed.

Example:

```text
persist:  canonical_price_color = AMBER
          semantic_contract_version = 1.0
render:   HOLDING
```

SQLite / PipelineSessionStore is **not implemented** in this increment.

## 5. Namespace / collision rules

English words are not primary IDs. Every collision below has a unique ID and an owner component.

| Collision | Distinct IDs |
| :--- | :--- |
| HOLD vs HOLDING | `ADAPTIVE_HOLD` ≠ `VIEWER_ADAPTIVE_HOLDING` ≠ `PRICE_HOLDING` (Arrivals alias for `PRICE_COLOR_AMBER`) |
| DELAYED | `VIEWER_ADAPTIVE_DELAYED` (infer-off / timeout) ≠ `PRICE_DELAYED` (`PRICE_COLOR_RED`) |
| FLAT | `ADAPTIVE_PATH_FLAT` ≠ `ADAPTIVE_FLAT` |
| RECOVERING | `FEED_RECOVERING` ≠ `PRICE_TENDENCY_RECOVERING` |
| TURNING_UP / TURNING_DOWN | `PRICE_PHASE_*` ≠ `PRICE_TENDENCY_*` |
| DEGRADED | `QUALITY_DEGRADED` (reserved bar enum) ≠ `FEED_DEGRADED` |
| INITIALIZING | Adaptive 15-bar ≠ Price 45-bar ≠ host status |
| persistence | D01 `ADAPTIVE_PERSISTENCE` ≠ Price cockpit `PRICE_COCKPIT_PERSISTENCE` |

`does_not_mean` is required wherever those collisions are plausible.

## 6. Quality lifecycle

`classifyAlpacaBar` writes only `COMPLETE`, `PARTIAL`, or `RECONSTRUCTED`. `csv_source` writes `COMPLETE`.

| ID | Lifecycle | Live path proven | Notes |
| :--- | :--- | :--- | :--- |
| `QUALITY_STALE` | RESERVED | NO | Proto mapper exists. No writer. `LiveFresh` is a 90s watermark, not this enum. |
| `QUALITY_DEGRADED` | RESERVED | NO | Proto mapper exists. No writer. Not `FeedState DEGRADED`. |

Reserved is an accurate classification, not a vague UNRESOLVED.

## 7. RK45 compatibility alias

Go production projection is **EXPM** (`time_term == false`).

| Surface | Classification |
| :--- | :--- |
| Canonical semantic ID | `PRICE_STATUS_PROJECTION_FAILURE` |
| Canonical current term | `PROJECTION_FAILURE` |
| Proto enum | `PRICING_STATUS_PROJECTION_FAILURE` |
| Skip reason | `PROJECTION_FAILURE` |
| `domain.PricingStatusProjectionFailure` stored string | `RK45_FAILURE` — **LEGACY FIXTURE COMPATIBILITY** for SADE Pricing Unit Run 001 CSV |
| `rk_success` / `RKSuccess` | boolean “projection succeeded”, not “RK45 ran” |
| Next.js status label | `PROJECTION_FAILURE` |

Do not change the frozen fixture string. Do not show RK45 as current Go science.

## 8. Three explanation levels

1. **Tooltip** — one sentence (`ui.tooltip`)
2. **Popover** — plain meaning plus important / does-not-mean notes
3. **Scientific meaning** — code-derived condition, or an explicit lifecycle (`RESERVED`) when the enum is not written

Public gRPC terms expose `go_symbol` and `proto_enum_or_field` only. Filesystem paths stay in the JSON for maintainers.

## 9. Implementation status

| Surface | Status |
| :--- | :--- |
| JSON contract v1.0 | Landed + cleanup closure |
| Go loader / validator | `internal/semantics` |
| SemanticService | `GetTerm`, `ListTerms`, `GetSemanticContract` |
| Startup policy | Load warning + service `Unavailable`; Adaptive/Pricing still start |
| Next.js cache | `/api/semantics` once; lookups local; refresh on version change |
| Tooltip / popover | `<SemanticHelp semanticId="…">` |
| Dictionary page | `/semantics` |
| Pipeline Session persistence | **Not started** — contract is ready |

## 10. Semantic authoring and build toolchain

**Authoring source:** `internal/semantics/catalog/v1.go`  
**Published canonical contract:** `internal/semantics/data/quantram_semantics_v1.json`  
**Runtime:** checked-in JSON → Go loader/validator → `SemanticService`. The server does not compile the Go catalog at runtime.

There is no Python semantic builder. `build_semantics_v1.py` was a one-time bootstrap and was removed.

Commands (`cmd/quantram-semantics`):

```text
go run ./cmd/quantram-semantics validate
go run ./cmd/quantram-semantics audit
go run ./cmd/quantram-semantics build
go run ./cmd/quantram-semantics build --check
```

| Command | Gate |
| :--- | :--- |
| `validate` | Hard fail on invalid catalog or published JSON |
| `build --check` | Hard fail if generated JSON ≠ checked-in JSON |
| `audit` | Diagnostic. Does not invent meanings. Does not fail CI on REVIEW/MISSING candidates |

`build` is deterministic (stable field order, 2-space indent, trailing newline, no timestamps).

### Future term workflow

1. Add the executable Go/proto/runtime state.
2. `go run ./cmd/quantram-semantics audit` — expect MISSING if the token is new.
3. Add a curated `semantics.Term` to `internal/semantics/catalog/v1.go`. Do not invent a meaning the code does not support.
4. `go run ./cmd/quantram-semantics build`
5. `go run ./cmd/quantram-semantics validate`
6. `go test ./internal/semantics ./internal/semantics/tooling ./internal/server`
7. Check in the updated JSON. SemanticService serves it. Next.js uses the semantic ID.

Do not increment the contract version for tooling. Increment it only when a term’s scientific meaning changes.

## 11. Source paths

```text
internal/semantics/catalog/v1.go
internal/semantics/tooling/
cmd/quantram-semantics/main.go
internal/semantics/data/quantram_semantics_v1.json
internal/semantics/{model,loader,validator,embed}.go
api/proto/quantram/v1/quantram.proto          SemanticService
internal/server/semantics.go
quantram-dashboard/src/lib/semantics.ts
quantram-dashboard/src/app/semantic-help.tsx
quantram-dashboard/src/app/semantics/page.tsx
```

## 12. Change log

| Date | Change |
| :--- | :--- |
| September 2, 2026 | Initial canonical baseline. Forensic inventory from current Go + viewer aliases. No scientific math changes. |
| September 2, 2026 | Cleanup closure: QUALITY_STALE/DEGRADED classified RESERVED; RK45 kept as fixture alias only; persist-canonical / render-presentation rule; presentation_only + canonical_source_ids. Version remains 1.0. |
| September 2, 2026 | Removed one-time `build_semantics_v1.py`. Canonical JSON unchanged. Future term edits are manual JSON + Go validation. |
| September 2, 2026 | Permanent Go catalog + `quantram-semantics` validate/audit/build. JSON key order normalized for deterministic encode. Meanings unchanged. Version remains 1.0. |
