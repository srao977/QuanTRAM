# SADE_Go Refactorability and Scaled Runtime Investigation

Date: August 27, 2026

Status: FORENSIC INVESTIGATION — HUMAN REVIEW REQUIRED

## Purpose

Establish, from actual executable code in the SADE and SDX repositories, how the
current system could evolve into a Go-dominant runtime (provisionally `SADE_Go`)
capable of processing thousands of concurrent, independently stateful
`IO_Vector` channels in near real time, and ultimately operating on Azure.

The investigation determines:

- what code exists today in each repository;
- which language owns each executable responsibility;
- which Python code is scientific mathematics and which is not;
- which mathematics can be faithfully reimplemented in Go;
- which mathematics has a justified reason to remain in Python;
- what the minimum Python residue would be;
- whether thousands-of-channel concurrency is architecturally feasible;
- how the resulting local runtime maps onto Azure.

This is an investigation and documentation task only. No code was created,
modified, refactored, or executed for scientific purposes.

## Scope

IN SCOPE:

- Full read of `C:\Users\chino\SADE` production Python and artifacts.
- Full read of `C:\Users\chino\SDX` handwritten Go, protobuf contract, tests,
  module manifests, and local Go module cache contents.
- Reconstruction of the actual current end-to-end runtime flow.
- Classification of every production-relevant module.
- Evidence-based Go/Python ownership boundary proposal.
- `IO_Vector` field-level analysis against existing structures.
- Concurrency, ordering, state, backpressure, and scale analysis.
- Local → Azure portability analysis.
- Hard-coding audit.
- Risk register and future validation strategy design.

OUT OF SCOPE (explicitly excluded, per task):

- Any code change to SADE or SDX.
- Any new code, including `SADE_Go` code.
- Any new scientific experiment or run.
- Go Decision Engine design or decision mathematics.
- BUY/SELL/HOLD rules, order rules, paper-order logic.
- Volume Pipeline development.
- Azure implementation.
- Web research (none was performed; all Go library findings come from the local
  module cache and repository manifests).

## Executive Summary

### What exists today

Two repositories hold a working, narrow, single-channel pipeline that has been
validated once end to end on a bounded 100-observation run.

`SDX` is a small Go gRPC service: **5 handwritten Go files, 833 lines**, plus
**1,122 lines of generated protobuf/gRPC code**, plus **967 lines of Go tests**.
It reads OHLCV rows from local CSV files and streams them as `MarketVector`
protobuf messages.

`SADE` is a pure Python package: **58 modules, 4,643 lines** of production
Python, plus **285 lines of generated protobuf Python**, plus **504 lines of
tests**. It contains no Go code whatsoever. It consumes the SDX stream over
gRPC and executes two scientific pipelines: an Adaptive Pipeline (D01 → D02 →
D04) and a Pricing Pipeline (causal derivatives → F4 ridge fit → RK45 →
PriceEngine → cockpit).

### What SDX currently owns

Source file reading (`internal/reader/reader.go`), per-source-partition bounded
queues, one producer goroutine per partition, fan-in to a single gRPC stream,
gRPC transport and streaming, partition/source status reporting, cancellation,
and graceful shutdown (`cmd/sdx-server/main.go`). SDX deliberately assigns no
timing semantics: `proto/sdx/v1/sdx.proto` line 63 states the source timestamp
is forwarded verbatim.

SDX is already a genuine, if minimal, Go concurrency runtime. Its `router.go`
already implements per-partition typed channels, fan-out to producers, fan-in
to a consumer, bounded-queue backpressure, and partition-level state — the
exact primitives the target architecture requires. Its test
`TestAAPLBackpressureDoesNotStopMSFT` (`internal/router/router_test.go:52`)
proves partition-level backpressure isolation already works at the
`RoutePartitions` layer.

### What SADE currently owns

Everything else: transport client, vector validation, field mapping, causal
ordering enforcement, all scientific mathematics, all scientific state, all
runtime state, serialization, configuration, summary aggregation, output
artifacts, CLI, and process lifecycle.

### Current Go/Python boundary

The boundary sits at exactly one place: the gRPC `StreamVectors` call. Go owns
the source side of that call; Python owns everything downstream of it. By
handwritten production line count the split is:

| | Lines | Share |
|---|---:|---:|
| Go (handwritten, SDX) | 833 | 15.2% |
| Python (handwritten, SADE) | 4,643 | 84.8% |

By runtime responsibility the Python share is higher still, because Go currently
owns only ingestion and transport, while Python owns orchestration, state,
mathematics, serialization, configuration and lifecycle.

### Broad Go-refactorability finding

The central and somewhat surprising finding of this investigation is that
**almost none of the SADE scientific code actually depends on Python's
scientific stack.**

- The entire Adaptive Pipeline — D01 (23 modules, 966 lines), D02 (4 modules,
  184 lines), D04 (7 modules, 239 lines), and the adaptive emitter (3 modules,
  452 lines) — imports **zero NumPy and zero SciPy**. Verified by import scan
  across `sade/`: the only non-stdlib imports in that entire subtree are
  `pydantic` (used for *validation*, not mathematics) in
  `sade/d04/models/envelope_context.py` and `sade/d04/models/capturability.py`.
  All adaptive mathematics is scalar arithmetic over `math.exp`, `math.sqrt`,
  `math.log1p`, `statistics.median`, `min`, `max` and comparisons.
- NumPy/SciPy appear in exactly **four files**, all in
  `sade/pricing_pipeline/`: `derivatives.py`, `dynamics.py`, `numerical.py`,
  `projection.py`, plus array plumbing in `pipeline.py`.
- The linear algebra actually used is small and narrow: one least-squares fit on
  a 15×3 design matrix (`np.linalg.lstsq`), one 4×4 linear solve plus one 30×4
  condition number (`np.linalg.solve`, `np.linalg.cond`), one 3×3 eigenvalue
  computation (`np.linalg.eigvals`), and one 3×3 matrix exponential
  (`scipy.linalg.expm`).

Every one of those five operations has a direct API in **gonum v0.17.0, which is
already fully extracted in this machine's local Go module cache** and already
hash-pinned in `SDX/go.sum` line 31 — including `mat.Dense.Exp`, which
implements the same Higham scaling-and-squaring Padé algorithm family that
`scipy.linalg.expm` uses.

The single genuine exception is RK45.

### Likely Python residue

**One module: `sade/pricing_pipeline/projection.py` — 146 lines — and within it
the single call to `scipy.integrate.solve_ivp(..., method="RK45")`.**

gonum has no initial-value ODE solver. Its `integrate` package contains only
`quad` (quadrature), and a recursive search of the extracted module tree for
`ode|ivp|runge|rk` package directories returns nothing relevant. There is no
adaptive Runge–Kutta–Fehlberg implementation anywhere in the local Go
dependency universe. Reproducing `solve_ivp`'s adaptive step-size controller,
per-component absolute tolerance vector, dense output at `t_eval`, and
`nfev`/`message` diagnostics is the only migration in this system that requires
writing genuinely new numerical infrastructure rather than calling an existing
library.

Projected distribution after a sensible migration:

| Classification | Modules | Lines | Share of current SADE Python |
|---|---:|---:|---:|
| G1 — trivial Go refactor (no float math) | 34 | 2,502 | 53.9% |
| G2 — straightforward Go mathematics | 20 | 1,650 | 35.5% |
| G3 — Go mathematics, equivalence validation required | 3 | 345 | 7.4% |
| P1 — retain Python initially | 1 | 146 | 3.1% |
| P2 — insufficient evidence | 0 | 0 | 0.0% |

The 90% Go / 10% Python direction is therefore not merely achievable — the
evidence suggests the realistic ceiling is closer to **96–97% Go**. The binding
constraint is not technical feasibility; it is the cost of scientific
equivalence validation and the willingness to write one Go RK45 integrator.

### Major migration risk

Not RK45. The highest-risk migration is **`sade/pricing_pipeline/dynamics.py::fit_f4`**,
because it forms and solves the ridge-regularised normal equations
`(XᵀX + λR)β = Xᵀy` directly (line 100), then reports `np.linalg.cond(design)`
(line 110). Normal-equation formation squares the condition number, so results
are genuinely sensitive to the LAPACK routine and pivoting order chosen. The
condition number then feeds `EmissionPolicy.emit` confidence thresholds
(`policy.py:196–201`) calibrated against hard-coded medians and 95th
percentiles. A small numerical difference in `cond` can flip a
HIGH/MEDIUM/LOW confidence classification, which flips emission colour. The
scientific output is *discretely* sensitive to a *continuously* small numerical
difference. That is the drift risk that matters.

### Two blockers that are independent of language

The investigation found two issues that would prevent thousands-of-channel
scale in **any** language and must be resolved before scale testing:

1. **Quadratic recomputation.** `PricingPipeline.process` recomputes the entire
   history on every observation. `causal_quadratic` refits every window from
   index `window-1` to the end of history (`derivatives.py:81`), and `fit_f4`
   refits every index from `window` to `len(p)-1` (`dynamics.py:89`) — but
   `pipeline.py` then reads only the single value at `active_index` (lines 277,
   293, 299). Per-observation cost therefore grows linearly with stream length,
   making total cost O(n²). Because each window fit depends only on its own
   trailing window, computing only the current index is *mathematically
   identical*. This is pure waste, not a scientific requirement.

2. **Unbounded state growth.** `AdaptiveEmitter` accumulates
   `self.emissions`, `self.initialization`, `self.adaptation_audit`, and
   `self.feedback_audit` forever, each entry a `deepcopy` of a large nested dict
   (`emitter.py:317, 323, 336, 362, 364`). `D01V02Model.trace_records` grows
   without bound (`model.py:333`). `AdaptivePipeline._rows` and every
   `PricingPipeline` history list grow without bound. The validated 100-vector
   run already produced 331 adaptation-audit entries and 170 feedback entries
   for 100 observations (`output/unit_runs/001/summary.json`). Multiplied by
   thousands of channels running continuously, this is an unbounded memory leak.

### Proposed SADE_Go runtime direction

Consolidate into a **single Go process** that owns ingestion, `IO_Vector`
representation, partitioning, ordering, concurrency, state, orchestration,
serialization, and — after phased migration — the mathematics. Expand SDX's
existing `router` pattern from "one partition per financial entity, one gRPC
stream" into "one owner goroutine per `IO_Vector` channel, N sources, typed
channels, fan-out/fan-in". Retain a narrow Python boundary for RK45 only, and
plan to close even that.

Critically: the current architecture crosses a process boundary and
protobuf-serialises **every single vector**. Consolidating ingestion into the Go
runtime removes that entire hop. This is the single largest structural
performance win available and it requires no scientific change at all.

### Is thousands-of-IO_Vector scale architecturally feasible?

**Yes — architecturally feasible, but not with the current code, and not yet
demonstrated.**

Goroutine and memory arithmetic is comfortable. SDX's existing pattern uses
2N+1 goroutines for N partitions; at N=1,000 that is ~2,001 goroutines, a
trivial load for the Go scheduler at roughly 4–16 MB of stack. Bounded
per-channel scientific state is on the order of 10–20 KB (a 15-record rolling
context plus a bounded price history of ~45 values across 5 arrays), giving
10–20 MB for 1,000 channels.

The feasibility is conditional on fixing the two blockers above, and on not
routing every observation through a cross-process Python call.

### What must be proven before Azure deployment

1. Numerical equivalence of every migrated mathematical function against the
   frozen Python baseline, at declared tolerance, including state trajectories
   over multi-hundred-observation replays — not just single-step outputs.
2. Per-channel causal ordering under concurrent load at target channel count.
3. Cross-channel failure and backpressure isolation (currently absent — see
   below).
4. Bounded memory under sustained multi-hour, multi-thousand-channel operation.
5. An actual latency budget. **No latency or throughput measurement exists
   anywhere in either repository.** A scan for `Benchmark`, `time.Since`,
   `latency`, `throughput`, `perf_counter` and `elapsed` found timing
   instrumentation in exactly one file — `emitter.py`, which records
   `component_lifecycle_ns` per stage but never aggregates or persists it. There
   are zero Go benchmarks. Every latency figure in this document is therefore
   marked NOT YET MEASURED.

## Final System Objective

Recorded here as the governing long-term objective against which this
investigation assesses the current code. This is the *target*, not a claim about
present behaviour.

SADE is to evolve into a scaled, Go-dominant runtime system capable of
processing thousands of independent `IO_Vector` channels concurrently in near
real time, end to end, ultimately operating on Azure. The intended system
should ingest multiple concurrent sources; convert or receive them as canonical
`IO_Vector`s; preserve causal ordering within each independently stateful
channel; process thousands of channels concurrently; maintain partitioned
runtime state; use Go as the primary runtime, concurrency, routing,
orchestration, state, transport, control and service language; move as much
scientific mathematics into Go as is technically sensible and scientifically
safe; retain Python only where justified; use narrow Go↔Python boundaries;
default to goroutines, typed channels, fan-out/fan-in and Pub/Sub-style flow;
avoid unnecessary worker-pool management; avoid unnecessary network
microservices; produce traceable Output `IO_Vector`s from Input `IO_Vector`s;
support later Volume and semantic paths and a later Go Decision Engine without
runtime redesign; and map naturally onto Azure messaging and runtime
infrastructure.

## Current Repositories Examined

| Repository | Path | Commit | Working tree at investigation start |
|---|---|---|---|
| SADE | `C:\Users\chino\SADE` | `037ee3b` "pricing_pipeline added and unit tested" | clean except untracked `newProposed_SADE082626.pptx` |
| SDX | `C:\Users\chino\SDX` | `1b68567` "design doc added" | `design_docs/SDX-ATEXIS_design_V1_0.md` modified; `cmd/`, `gen/`, `internal/`, `proto/`, `go.mod`, `go.sum`, `buf.*`, `README.md`, `implementation_docs/` all untracked |

Note: the entire SDX Go implementation is **untracked** in git. Only the design
docs are committed. This is a provenance risk recorded in the risk register.

### SADE file inventory (production Python, exact line counts)

| Module | Lines |
|---|---:|
| `sade/__init__.py` | 34 |
| `sade/__main__.py` | 72 |
| `sade/adaptive_pipeline/__init__.py` | 45 |
| `sade/adaptive_pipeline/pipeline.py` | 386 |
| `sade/adaptive_emitter/__init__.py` | 34 |
| `sade/adaptive_emitter/emitter.py` | 339 |
| `sade/adaptive_emitter/normalizer.py` | 79 |
| `sade/configuration/scientific_baseline.py` | 48 |
| `sade/input/__init__.py` | 33 |
| `sade/input/sdx_client.py` | 145 |
| `sade/d01/__init__.py` | 2 |
| `sade/d01/v02/__init__.py` | 4 |
| `sade/d01/v02/adaptation.py` | 25 |
| `sade/d01/v02/coherence.py` | 11 |
| `sade/d01/v02/config.py` | 130 |
| `sade/d01/v02/forward.py` | 22 |
| `sade/d01/v02/half_life.py` | 13 |
| `sade/d01/v02/health.py` | 26 |
| `sade/d01/v02/innovation.py` | 7 |
| `sade/d01/v02/kinematics.py` | 27 |
| `sade/d01/v02/model.py` | 332 |
| `sade/d01/v02/observations.py` | 45 |
| `sade/d01/v02/outputs.py` | 51 |
| `sade/d01/v02/persistence.py` | 10 |
| `sade/d01/v02/perturbation.py` | 65 |
| `sade/d01/v02/reference.py` | 12 |
| `sade/d01/v02/reversal.py` | 29 |
| `sade/d01/v02/snapshot.py` | 32 |
| `sade/d01/v02/state.py` | 39 |
| `sade/d01/v02/strength.py` | 25 |
| `sade/d01/v02/trace.py` | 24 |
| `sade/d01/v02/uncertainty.py` | 24 |
| `sade/d01/v02/volume.py` | 11 |
| `sade/d02/__init__.py` | 2 |
| `sade/d02/v02/__init__.py` | 3 |
| `sade/d02/v02/builder.py` | 94 |
| `sade/d02/v02/models.py` | 85 |
| `sade/d04/__init__.py` | 34 |
| `sade/d04/envelope/__init__.py` | 3 |
| `sade/d04/envelope/capturability_model.py` | 78 |
| `sade/d04/models/__init__.py` | 7 |
| `sade/d04/models/capturability.py` | 11 |
| `sade/d04/models/enums.py` | 36 |
| `sade/d04/models/envelope_context.py` | 70 |
| `sade/pricing_pipeline/__init__.py` | 35 |
| `sade/pricing_pipeline/pipeline.py` | 372 |
| `sade/pricing_pipeline/derivatives.py` | 120 |
| `sade/pricing_pipeline/dynamics.py` | 105 |
| `sade/pricing_pipeline/numerical.py` | 120 |
| `sade/pricing_pipeline/projection.py` | 146 |
| `sade/pricing_pipeline/price_engine/__init__.py` | 50 |
| `sade/pricing_pipeline/price_engine/contracts.py` | 171 |
| `sade/pricing_pipeline/price_engine/engine.py` | 101 |
| `sade/pricing_pipeline/price_engine/policy.py` | 252 |
| `sade/pricing_pipeline/price_engine/cockpit.py` | 248 |
| `sade/unit_run/__init__.py` | 1 |
| `sade/unit_run/run_001.py` | 102 |
| `sade/unit_run/run_pricing_001.py` | 216 |
| **Total production Python** | **4,643** |

Additional SADE files:

| Category | Files | Lines |
|---|---:|---:|
| Generated protobuf Python (`sade/input/generated/sdx/v1/`) | 5 | 285 |
| Tests (`tests/`) | 3 | 504 |
| Go source | **0** | **0** |

Non-code artifacts: `output/unit_runs/001/` (observations.csv 48 KB,
summary.json, unit_run_001_with_independence_summary.json,
migration_hash_evidence.json 22 KB — a JSON array of per-module source→target
SHA-256 migration records), `output/unit_runs/pricing_001/` (observations.csv
9.8 KB, pricing_summary.json, migration_equivalence.json),
`docs/design/`, `docs/implementations/`, `docs/runs/`,
`newProposed_SADE082626.pptx`.

### SDX file inventory (exact line counts)

| File | Lines | Category |
|---|---:|---|
| `cmd/sdx-server/main.go` | 76 | runtime entry point |
| `cmd/sdx-client/main.go` | 176 | validation/fidelity client |
| `internal/reader/reader.go` | 147 | source reading |
| `internal/router/router.go` | 270 | partitioning, concurrency, fan-in |
| `internal/server/server.go` | 164 | gRPC service implementation |
| **Total handwritten Go** | **833** | |
| `gen/sdx/v1/sdx.pb.go` + `sdx_grpc.pb.go` | 1,122 | generated |
| `internal/reader/reader_test.go` | 150 | test |
| `internal/router/router_test.go` | 185 | test |
| `internal/server/server_test.go` | 632 | test |
| **Total Go tests** | **967** | |

Contract: `proto/sdx/v1/sdx.proto` (110 lines), `buf.yaml`, `buf.gen.yaml`.
Manifests: `go.mod` (16 lines), `go.sum` (38 lines).
Data: `data_sources/Stocks/` — 5 CSV files, ~51 MB total.
Docs: `design_docs/SDX-ATEXIS_design_V1_0.md`, `design_docs/backpressure_producer_consumer.md`,
four `implementation_docs/` phase reports.

## Executable Evidence Authority

This investigation treats **executable code as authoritative** for current
behaviour. Markdown documents in either repository were read for intent only and
are never cited as evidence of runtime behaviour.

One example of why this matters: `SDX/design_docs/backpressure_producer_consumer.md`
presents a three-tier `asyncio.Queue` producer/consumer design in **Python**,
with a hard-coded `file_map` of five instrument symbols and a pre-processor that
"strips original temporal records and stamps them with the system's real-time
execution clock". **None of this is implemented.** The actual SDX
implementation is Go, uses buffered Go channels rather than `asyncio.Queue`, and
does the exact opposite on timestamps — `reader.go:157` forwards
`record[0]` verbatim and `sdx.proto:63` documents that SDX assigns no timing
semantics. That document describes an alternative that was not built. It is
recorded here as context, and excluded from all findings.

Where this document states a behaviour, it cites repository, file path, and
function or line.

## Inputs

- SADE production Python source: 58 modules, read in full.
- SADE generated protobuf Python bindings: inspected.
- SADE test suite: 3 modules, read in full.
- SADE output artifacts: `summary.json`, `pricing_summary.json`,
  `migration_hash_evidence.json`, observation CSV headers.
- SADE `pyproject.toml` declared dependencies.
- Installed Python environment: Python 3.13.7, NumPy 2.5.1, SciPy 1.18.0,
  pydantic 2.13.4, grpcio 1.78.0.
- SDX handwritten Go source: 5 files, read in full.
- SDX generated Go bindings: inspected.
- SDX Go test suite: 3 files, read for concurrency and ordering guarantees.
- SDX `proto/sdx/v1/sdx.proto`: read in full.
- SDX `go.mod`, `go.sum`, and full module graph via `go list -m all`.
- Local Go module cache at `C:\Users\chino\go\pkg\mod`: gonum package inventory
  and API surface inspected.
- Git state of both repositories.

## Outputs

This single investigation document. No other artifact was produced.

## Assumptions

1. The bounded 100-observation run recorded in `output/unit_runs/` is the
   current validated scientific baseline. Its numerical outputs are treated as
   the reference any future Go implementation must reproduce.
2. `migration_hash_evidence.json` records a completed prior migration from an
   external `APTF` repository into SADE, and SADE is now runtime-independent of
   it — supported by the audit-hook evidence in `run_001.py` /
   `run_pricing_001.py` and the observed `aptf_modules_loaded_count: 0`.
3. The `APTF` repository at `C:/Users/chino/APTF` is a historical scientific
   authority used only by `tests/test_pricing_migration_equivalence.py`, which
   skips when it is absent (line 78–79). It is not a runtime dependency.
4. Scientific constants and coefficients in `sade/d01/v02/config.py`,
   `PolicyConfig`, and `CockpitPolicyConfig` are frozen calibration, not tunable
   configuration, unless a human decides otherwise.
5. gonum's availability in the local module cache means adopting it requires a
   `go.mod` declaration change but **not** a network fetch.
6. "Near real time" has no numerical SLA yet. This document supplies a latency
   budget framework, not targets.

## Explicit Exclusions

- No SADE code was modified.
- No SDX code was modified.
- No tests were modified.
- No scientific runs were executed.
- No new code of any kind was written.
- No `SADE_Go` code was created.
- No repository cleanup, file deletion, or state reset was performed.
- No Volume Pipeline design.
- No Go Decision Engine design, decision mathematics, order rules, or
  BUY/SELL/HOLD logic.
- No Azure implementation.
- No web research.
- No named financial instrument is used as an architectural example anywhere in
  the proposed design. Where instrument symbols appear, they are quotations of
  existing code.

---

# Part A — Repository Inventory

## A.1 SADE inventory by responsibility

| Area | Location | Present? | Notes |
|---|---|---|---|
| Python packages | `sade/` | Yes | 58 production modules |
| Go code | — | **No** | zero `.go` files |
| protobuf/gRPC definitions | — | No `.proto`; consumes SDX contract | generated bindings vendored at `sade/input/generated/sdx/v1/` |
| Unit-run code | `sade/unit_run/run_001.py`, `run_pricing_001.py` | Yes | 318 lines; validation harness, not product runtime |
| Adaptive Pipeline | `sade/adaptive_pipeline/pipeline.py` | Yes | 386 lines; orchestration only |
| Adaptive emitter | `sade/adaptive_emitter/emitter.py`, `normalizer.py` | Yes | 418 lines; scientific sequencing + state |
| Pricing Pipeline | `sade/pricing_pipeline/` | Yes | 898 lines across 6 modules |
| Migrated Price Engine | `sade/pricing_pipeline/price_engine/` | Yes | 822 lines across 5 modules |
| RK45 | `sade/pricing_pipeline/projection.py::solve_cover` | Yes | SciPy `solve_ivp` |
| F4 | `sade/pricing_pipeline/dynamics.py::fit_f4` | Yes | NumPy ridge normal equations |
| Derivative code | `sade/pricing_pipeline/derivatives.py` | Yes | `causal_quadratic`, `derivative_state` |
| D01 | `sade/d01/v02/` | Yes | 23 modules, 966 lines, **stdlib only** |
| D02 | `sade/d02/v02/` | Yes | 4 modules, 184 lines, **stdlib only** |
| D04 | `sade/d04/` | Yes | 7 modules, 239 lines, pydantic for validation only |
| D03 | — | No | explicitly excluded in `sade/d04/__init__.py` docstring |
| Volume code | `sade/d01/v02/volume.py` (11 lines) | Partial | volume *influence* inside D01 only; no Volume Pipeline |
| Scientific state | `sade/d01/v02/state.py`, emitter attributes, policy/cockpit state | Yes | see Part G |
| Pipeline state | `AdaptivePipeline._expected_index/_rows`, `PricingPipeline` history lists | Yes | unbounded |
| SDX client integration | `sade/input/sdx_client.py` | Yes | 145 lines |
| Serialization | `pipeline.py::_write_csv`, `summary` dict, `as_dict()` methods, `canonical_sha256` | Yes | CSV + JSON |
| Configuration | `sade/d01/v02/config.py`, `sade/configuration/scientific_baseline.py`, dataclass configs | Yes | all in-code; no config files |
| Tests | `tests/` | Yes | 3 modules, 504 lines |
| Runtime entry points | `sade/__main__.py`, `run_001.py`, `run_pricing_001.py` | Yes | CLI only; no service |
| Documentation | `docs/design/`, `docs/implementations/`, `docs/runs/` | Yes | intent only |
| Output artifacts | `output/unit_runs/001/`, `output/unit_runs/pricing_001/` | Yes | preserved, not modified |
| Declared dependencies | `pyproject.toml` | grpcio, pydantic, numpy, scipy; dev: pytest | |

### Responsibility confirmed by reading code, not filenames

Two cases where the filename is misleading and code reading was necessary:

- `sade/d01/v02/volume.py` sounds like a Volume Pipeline. It is 11 lines
  computing a single scalar volume-influence term `v* = log1p(volume/ref) +
  log1p(volume)/10`, bounded to `[0,3]`, feeding D01's `effective_mass`
  (`model.py:116–121, 153`). It is *not* a Volume path.
- `sade/pricing_pipeline/derivatives.py::derivative_state` (lines 98–131) looks
  like a live pricing stage. It is **never called in production**:
  `pipeline.py:55` imports only `causal_quadratic`. Dead code in the production
  path, though harmless.

## A.2 SDX inventory by responsibility

| Area | Location | Present? | Notes |
|---|---|---|---|
| Go packages | `internal/reader`, `internal/router`, `internal/server` | Yes | 581 lines |
| cmd entry points | `cmd/sdx-server` (76), `cmd/sdx-client` (176) | Yes | server + validation client |
| gRPC server | `internal/server/server.go` | Yes | two services registered |
| protobuf definitions | `proto/sdx/v1/sdx.proto` | Yes | 110 lines, 2 services, 8 messages, 3 enums |
| Readers | `internal/reader/reader.go::SDReader` | Yes | CSV only, fixed header |
| Routers | `internal/router/router.go::SDXRouter` | Yes | partitions, fan-in, status |
| Source handling | `SDReader.Read` | Yes | opens file, validates header, parses rows |
| MarketVector generation | `reader.go::parseVector` | Yes | |
| Streaming | `server.go::StreamVectors` | Yes | server-streaming RPC |
| Concurrency | `router.go::RoutePartitions`, `Route` | Yes | goroutine per partition + per forwarder |
| Channel usage | `chan *sdxv1.MarketVector` buffered at `DefaultCapacity`=10; unbuffered fan-in channel | Yes | |
| Goroutine usage | 1 producer + 1 forwarder per partition, + 1 collector, + 1 serve loop | Yes | 2N+2 |
| Synchronization | `sync.RWMutex` in both `SDReader` and `SDXRouter`; `sync.WaitGroup` for forwarders | Yes | |
| State | `SDReader.status`, `SDXRouter.statuses`, `SDXRouter.configured` | Yes | mutable, shared |
| Error handling | typed sentinel errors, gRPC status codes | Yes | `server.go:43–52` |
| Lifecycle | `signal.NotifyContext`, `GracefulStop` with 5s timeout then `Stop` | Yes | `main.go:55–80` |
| Shutdown | Yes | | |
| Configuration | `GRPC_PORT` and `SDX_<ENTITY>_CSV` env vars only | Partial | entity *set* is compile-time |
| Tests | 3 files, 967 lines | Yes | includes real concurrency tests |
| Source-specific assumptions | fixed CSV header `[timestamp,open,high,low,close,volume]` (`reader.go:16`) | Yes | |
| Hard-coded assumptions | 5 instrument symbols compiled in (`main.go:23`); filename pattern (`main.go:31`) | Yes | see Part K |

### Is SDX already close to a generic Go ingress runtime?

Partly — closer than expected on concurrency, further than expected on
genericity. Assessment deferred to Part H.4 after the evidence is laid out.

---

# Part B — Current End-to-End Flow

Reconstructed from executable code only.

## B.1 Actual call sequence

1. **`cmd/sdx-server/main.go`** starts. It builds a `map[string]*reader.SDReader`
   over a compile-time slice of five instrument symbols (line 23), deriving each
   CSV path as `"data_sources/Stocks/" + entity + "_1min_firstratedata.csv"`,
   overridable per entity via `SDX_<ENTITY>_CSV` (lines 30–35). It listens on
   `:50051` (overridable via `GRPC_PORT`) and registers both
   `SDXDataServiceServer` and `SDXControllerServiceServer`.

2. **SADE** is launched separately, as a *Python CLI process*, not a service:
   `python -m sade run` or `python -m sade.unit_run.run_pricing_001`.
   `sade/__main__.py` constructs `AdaptivePipelineConfig` and enters
   `AdaptivePipeline`.

3. **`sade/input/sdx_client.py::SadeSdxClient.stream_vectors`** opens an
   insecure gRPC channel (line 96), creates both stubs, and issues
   `StreamVectors(StreamRequest{entities, max_vectors_per_entity})` with a
   timeout. It yields protobuf `MarketVector` objects.

4. **`internal/server/server.go::StreamVectors`** validates the request
   (non-empty entities, upper-cased, no duplicates, supported, non-zero count),
   derives a cancellable context from the stream context, and calls
   `router.Route`.

5. **`internal/router/router.go::RoutePartitions`** creates one buffered channel
   of capacity 10 per requested entity, marks each partition ACTIVE, and spawns
   one producer goroutine per entity running `SDReader.Read`.

6. **`internal/reader/reader.go::Read`** opens the CSV, validates the header
   against the fixed expected header, then loops `maxVectors` times: read
   record, `parseVector` into a `MarketVector` with `SourceRowIndex` =
   loop counter, update `VectorsRead`, then `select` on `output <- vector` or
   `ctx.Done()`. The blocking send on a full buffered channel is the
   backpressure mechanism. After the loop it probes one more row to distinguish
   EXHAUSTED from COMPLETED.

7. **`router.Route`** spawns one forwarder goroutine per partition, each
   draining its own partition channel in order and sending into a single
   **unbuffered** `vectors` channel, incrementing `vectorsRouted`. A collector
   goroutine waits for all forwarders, then closes `vectors` and forwards the
   first producer error.

8. **`server.StreamVectors`** ranges over `vectors` and calls `stream.Send` for
   each. On completion it reads the result channel and maps errors to gRPC codes
   (`Canceled`/`DeadlineExceeded` from context, `NotFound` for
   `os.ErrNotExist`, otherwise `Internal`).

9. Back in Python, **`AdaptivePipeline.process_vector`** (per vector):
   - `_validate_vector` checks presence of 8 required attributes, entity match,
     and strict `source_row_index == expected_index` (lines 110–132);
   - `build_source_row` maps the vector into a `dict[str,str]`, injecting
     `data_valid="true"` and `session_type="UNKNOWN"` as declared SADE
     assumptions (lines 96–107);
   - `physical_row_from_source_index` adds 2 (line 67);
   - calls `AdaptiveEmitter.process`.

10. **`AdaptiveEmitter.process`** (`emitter.py:176–370`):
    - `SourceRowNormalizer.source_row_to_normalized_observation` parses the ISO
      timestamp to epoch seconds and builds a `NormalizedObservation`, setting
      `receive_time = event_time` (normalizer.py:68);
    - enforces strictly increasing source time (line 195–196);
    - snapshots `state_before` including a D01 state hash;
    - `D01V02Model.step(observation)` → `(DMOOutput, FMOOutput)`;
    - `build_return_shape(dmo, fmo)` → `ReturnShape` (D02);
    - `CapturabilityModelV0_2.evaluate(shape, EnvelopeContext.production(...))`
      → `CapturabilityResult` (D04), giving `H, Q_G, Q_S, Q_R, C`;
    - if fewer than 15 observations completed: status INITIALIZING, no decision;
      otherwise ACTIONABLE, computes rolling-15 adaptive properties and applies
      `_decide` → BUY/SELL/HOLD;
    - updates position state, appends to the rolling 15-deque, writes adaptation
      and feedback audit entries, computes `emission_id` and a verification hash
      via `canonical_sha256`, and returns an immutable emission dict.

11. **`AdaptivePipeline._build_record`** flattens the emission into a CSV row.

12. In the integrated run only, **`sade/unit_run/run_pricing_001.py:119`** feeds
    that adaptive row directly into **`PricingPipeline.process`**:
    - validates 8 required fields, entity match, and strict source-order
      increment;
    - appends to 8 parallel history lists, converting the timestamp to minutes;
    - sets `active_index = len-2` — the pricing stage deliberately operates one
      observation **behind** the newest, because `jp` requires `p2[i]` and
      `p2[i-1]`;
    - `causal_quadratic(times, closes, 15)` → `p1`, `p2` over all history;
    - if `p1`/`p2` non-finite at `active_index`: WARMUP_DERIVATIVE;
    - computes `jp[i] = p2[i] - p2[i-1]` over all history;
    - if `active_index < f4_window` or `jp` window not all finite: WARMUP_F4;
    - `fit_f4(p, p1, p2, jp, 30, 1.0)` over all history;
    - `valid_fit` check → F4_FIT_UNAVAILABLE;
    - `solve_cover([active_index], ...)` → RK45 one-minute projection;
    - `build_numerical_row(...)` → adds 3×3 eigenvalues and matrix-exponential
      amplification;
    - `PriceEngine.observe(observation, numerical, policy_state)` → validates
      symbol/timestamp coherence, delegates to `EmissionPolicy.emit` →
      `PriceEmission` + next `PolicyState`;
    - `PriceCockpitInterpreter.observe(emission, cockpit_state)` →
      `PriceCockpitEmission` + next `CockpitState`.

13. Outputs are written: `observations.csv`, `summary.json`,
    `pricing_summary.json`, `migration_equivalence.json`.

## B.2 Boundary characterisation

| Property | Current behaviour | Evidence |
|---|---|---|
| Process boundaries | Exactly 2 OS processes: SDX Go server, SADE Python CLI | `cmd/sdx-server/main.go`, `sade/__main__.py` |
| Language boundaries | Exactly 1: the gRPC stream | `sdx_client.py:133` |
| gRPC boundaries | 1 data service (server-streaming), 1 controller service (3 unary RPCs) | `sdx.proto:7–15` |
| Objects passed | `StreamRequest` → stream of `MarketVector`; controller request/response messages | `sdx.proto:49–110` |
| State ownership | Source/partition state in Go; all scientific + pipeline state in Python | Part G |
| Call direction | Strictly SADE → SDX. SDX never calls SADE. | no client code in SDX except validation CLI |
| Synchronous/asynchronous | Go side asynchronous (goroutines); Python side fully synchronous, single-threaded, blocking iteration | `pipeline.py:300` `for vector in stream:` |
| Current concurrency | Go: N producers + N forwarders concurrent. Python: **none** — one thread, one entity, one pipeline instance | `AdaptivePipelineConfig.entity` is a single string |
| Current ordering | Per-partition order preserved by construction and asserted by tests; cross-partition interleaving non-deterministic | `router_test.go`, `server_test.go:101–151` |
| Source assumptions | local CSV, fixed 6-column header, 5 compile-time symbols, per-request row index restarting at 0 | `reader.go:16,69`; `main.go:23` |
| Failure behaviour | Any producer error calls the **shared** `cancel()`, aborting *all* partitions in the request | `router.go:240–243` |

Two failure-behaviour findings that materially affect the target architecture:

- **Cross-channel failure coupling.** `router.go:242` calls `cancel()` on the
  route-wide context when *any* single partition's source fails. One bad source
  therefore terminates every other channel in the same request. The test
  `TestCancellationTerminatesAllBlockedPartitions`
  (`router_test.go:134`) asserts exactly this behaviour — it is intentional for
  a bounded validation run, and unacceptable for a thousands-channel runtime.

- **Cross-channel head-of-line blocking at the fan-in.** While
  `RoutePartitions` isolates partitions, `Route` merges them into a single
  **unbuffered** channel (`router.go:183`) consumed by one `stream.Send` loop.
  If the gRPC consumer is slow, every forwarder blocks. So partition isolation
  exists one layer down but is destroyed at the layer the gRPC service actually
  uses. `TestAAPLBackpressureDoesNotStopMSFT` tests `RoutePartitions`, not
  `Route` — the isolation it proves is not the isolation the service delivers.

## DIAGRAM 1 — ACTUAL CURRENT SDX → SADE RUNTIME

```text
                        ── PROCESS 1: SDX (Go) ──

  data_sources/Stocks/<SYMBOL>_1min_firstratedata.csv     (5 files, compiled-in symbol list)
        │  os.Open + csv.Reader, fixed header check
        ▼
  SDReader.Read  (1 goroutine per partition)
        │  parseVector -> *MarketVector{entity_id, source_row_index, OHLCV, source_timestamp}
        │  select { output <- vector | <-ctx.Done() }        <-- blocking send = backpressure
        ▼
  partition channel  chan *MarketVector  cap=10   (one per entity)
        │
        │   ... one per configured entity ...
        ▼
  forwarder goroutines (1 per partition)  ──┐
                                            │  FAN-IN
                                            ▼
                            vectors chan *MarketVector  cap=0  (UNBUFFERED)
                                            │
                                            ▼
                            server.StreamVectors: for v := range vectors { stream.Send(v) }
                                            │
        ════════════════════════════════════╪════════════════════════════════════
                          gRPC server-stream │ protobuf serialise EVERY vector
        ════════════════════════════════════╪════════════════════════════════════
                                            │
                        ── PROCESS 2: SADE (Python) ──
                                            ▼
                    SadeSdxClient.stream_vectors  (generator, blocking)
                                            │
                                            ▼
                    AdaptivePipeline.process_vector          [SINGLE THREAD]
                      _validate_vector      (entity + strict source_row_index)
                      build_source_row      (+ data_valid="true", session_type="UNKNOWN")
                      physical_row = source_row_index + 2
                                            │
                                            ▼
                    AdaptiveEmitter.process
                      SourceRowNormalizer -> NormalizedObservation (receive_time := event_time)
                      D01V02Model.step      -> DMOOutput, FMOOutput
                      build_return_shape    -> ReturnShape                 (D02)
                      CapturabilityModelV0_2.evaluate -> H, Q_G, Q_S, Q_R, C  (D04)
                      rolling-15 context -> adaptive properties -> BUY/SELL/HOLD
                      canonical_sha256 x3   -> observation_id, emission_id, verify hash
                                            │
                                            ▼
                    _build_record  -> flattened dict  -> _rows[]  (UNBOUNDED)
                                            │
                                            │  (integrated run only: run_pricing_001.py:119)
                                            ▼
                    PricingPipeline.process   active_index = len(history) - 2
                      causal_quadratic  (np.linalg.lstsq, ALL windows)   -> p1, p2
                      jp[i] = p2[i]-p2[i-1]                (ALL history)
                      fit_f4  (np.linalg.solve 4x4, np.linalg.cond 30x4, ALL indices)
                      solve_cover (scipy solve_ivp RK45, horizon [0,1] min, t_eval 11 pts)
                      build_numerical_row (np.linalg.eigvals 3x3, scipy.linalg.expm 3x3)
                      PriceEngine.observe -> EmissionPolicy.emit -> PriceEmission
                      PriceCockpitInterpreter.observe        -> PriceCockpitEmission
                                            │
                                            ▼
              output/unit_runs/**  observations.csv, summary.json, pricing_summary.json
```

## DIAGRAM 2 — CURRENT LANGUAGE / PROCESS BOUNDARIES

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ OS PROCESS 1 — Go 1.25.0 — module "sdx" — 833 handwritten LOC                │
│                                                                              │
│  OWNS:  file I/O · CSV parse · MarketVector construction · partition queues   │
│         goroutines · per-partition ordering · bounded-queue backpressure      │
│         fan-in · gRPC transport · status reporting · cancellation · shutdown  │
│                                                                              │
│  DEPS:  google.golang.org/grpc 1.80.0 · google.golang.org/protobuf 1.36.11    │
│         (gonum 0.17.0 present in go.sum + module cache, NOT declared/imported)│
│  STATE: SDReader.status (RWMutex) · SDXRouter.statuses/configured (RWMutex)   │
│  CONFIG: GRPC_PORT, SDX_<ENTITY>_CSV   —   entity SET is compile-time         │
└──────────────────────────────────────────────────────────────────────────────┘
                    ▲
                    │  ONLY LANGUAGE BOUNDARY IN THE SYSTEM
                    │  gRPC/HTTP2, insecure credentials, localhost:50051
                    │  1 server-streaming RPC + 3 unary control RPCs
                    │  protobuf marshal + unmarshal PER VECTOR
                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ OS PROCESS 2 — CPython 3.13.7 — package "sade" — 4,643 production LOC        │
│                                                                              │
│  OWNS:  transport client · vector validation · field mapping · causal order   │
│         ALL scientific mathematics · ALL scientific state · ALL runtime state │
│         orchestration · serialization · configuration · summary · CLI         │
│                                                                              │
│  DEPS:  grpcio 1.78.0 · pydantic 2.13.4 · numpy 2.5.1 · scipy 1.18.0          │
│  CONCURRENCY: NONE — single thread, single entity, single pipeline instance   │
│  STATE: unbounded in-memory lists + rolling deques + dataclass state objects  │
│  CONFIG: entirely in-code (frozen dataclasses); no config files              │
└──────────────────────────────────────────────────────────────────────────────┘

LOC:  Go 833 handwritten (15.2%)   |   Python 4,643 handwritten (84.8%)
      Go 1,955 incl. generated      |   Python 4,928 incl. generated
```

---

# Part C — Current Adaptive Pipeline Forensics

## C.1 Module-level classification

Every module reachable from `AdaptivePipeline`, classified per the required
taxonomy.

| Module | Function/class | Classification |
|---|---|---|
| `adaptive_pipeline/pipeline.py` | `physical_row_from_source_index` | DATA MAPPING |
| | `build_source_row` | DATA MAPPING |
| | `_validate_vector` | VALIDATION |
| | `_build_record` | DATA MAPPING / SERIALIZATION |
| | `AdaptivePipelineConfig` | CONFIGURATION |
| | `AdaptivePipeline.process_vector` | PIPELINE ORCHESTRATION |
| | `AdaptivePipeline.run` | PIPELINE ORCHESTRATION + TRANSPORT + aggregation |
| | `AdaptivePipeline._write_csv` | SERIALIZATION |
| | `AdaptivePipeline.close/__enter__/__exit__` | OTHER (lifecycle) |
| `adaptive_emitter/normalizer.py` | `SourceRowNormalizer.source_row_to_normalized_observation` | DATA MAPPING |
| `adaptive_emitter/emitter.py` | `canonical_sha256` | SERIALIZATION |
| | `DevelopmentObservationStream` | DIAGNOSTIC / TEST (unused in production) |
| | `AdaptiveEmitter._adaptive_properties` | SCIENTIFIC MATHEMATICS |
| | `AdaptiveEmitter._decide` | SCIENTIFIC MATHEMATICS (decision predicate) |
| | `AdaptiveEmitter.process` | PIPELINE ORCHESTRATION + SCIENTIFIC STATE |
| | `AdaptiveEmitter.context/position_state/audits` | SCIENTIFIC STATE |
| `configuration/scientific_baseline.py` | `get_baseline_fingerprints` | CONFIGURATION |
| `input/sdx_client.py` | all | TRANSPORT |
| `d01/v02/reference.py` | `update_reference_and_scale` | SCIENTIFIC MATHEMATICS |
| `d01/v02/kinematics.py` | `compute_kinematics`, `_clip` | SCIENTIFIC MATHEMATICS |
| `d01/v02/innovation.py` | `innovation_magnitude` | SCIENTIFIC MATHEMATICS |
| `d01/v02/volume.py` | `update_volume_influence` | SCIENTIFIC MATHEMATICS |
| `d01/v02/coherence.py` | `compute_coherence` | SCIENTIFIC MATHEMATICS |
| `d01/v02/strength.py` | `compute_strength`, `_sigmoid` | SCIENTIFIC MATHEMATICS |
| `d01/v02/persistence.py` | `update_persistence` | SCIENTIFIC MATHEMATICS |
| `d01/v02/uncertainty.py` | `compute_uncertainty` | SCIENTIFIC MATHEMATICS |
| `d01/v02/reversal.py` | `compute_reversal_propensity` | SCIENTIFIC MATHEMATICS |
| `d01/v02/perturbation.py` | `_direction`, `infer_perturbation_class`, `classify_perturbation` | SCIENTIFIC MATHEMATICS |
| `d01/v02/half_life.py` | `adapt_half_life` | SCIENTIFIC MATHEMATICS |
| `d01/v02/adaptation.py` | `update_parameters` | SCIENTIFIC MATHEMATICS |
| `d01/v02/forward.py` | `compute_forward_interval`, `forward_samples`, `propagate_level` | SCIENTIFIC MATHEMATICS |
| `d01/v02/health.py` | `evaluate_health` | VALIDATION (mutates counters) |
| `d01/v02/model.py` | `D01V02Model.step` | PIPELINE ORCHESTRATION (scientific sequencing) |
| | `D01V02Model._state_hash` | SERIALIZATION |
| | `D01V02Model.snapshot` | SERIALIZATION |
| `d01/v02/state.py` | `StateVector`, `HalfLifeState`, `RuntimeState` | SCIENTIFIC STATE |
| `d01/v02/observations.py` | `NormalizedObservation` | DATA MAPPING |
| | `assert_causal_sequence` | VALIDATION |
| `d01/v02/outputs.py` | `DMOOutput`, `FMOOutput`, `FMOSample` | SERIALIZATION |
| `d01/v02/config.py` | all 15 config dataclasses | CONFIGURATION |
| `d01/v02/snapshot.py` | `to_snapshot`, `from_snapshot`, `state_hash` | SERIALIZATION |
| `d01/v02/trace.py` | `TraceRecord` | DIAGNOSTIC |
| `d02/v02/builder.py` | `_require_finite`, `_require_bounded`, `_validate_input` | VALIDATION |
| | `build_return_shape` | SCIENTIFIC MATHEMATICS |
| `d02/v02/models.py` | `ForwardSample`, `ReturnShape`, `PathDirection` | SCIENTIFIC STATE + VALIDATION |
| `d04/envelope/capturability_model.py` | `validate_return_shape` | VALIDATION |
| | `geometry_quality`, `structural_quality`, `risk_quality`, `evaluate` | SCIENTIFIC MATHEMATICS |
| `d04/models/envelope_context.py` | `EnvelopeContext`, provenance validator | CONFIGURATION + VALIDATION |
| `d04/models/capturability.py` | `CapturabilityResult` | SERIALIZATION + VALIDATION |
| `d04/models/enums.py` | 6 enums | OTHER (vocabulary; 4 of 6 unused in V0.1) |

**Aggregate:** of 4,643 production Python lines, mathematics and scientific
state account for roughly 1,995 lines (43%). The remaining 2,648 lines (57%) are
orchestration, mapping, validation, serialization, transport, configuration,
lifecycle and diagnostics — i.e. responsibilities the target architecture
assigns to Go regardless of any scientific decision.

## C.2 Scientific mathematics detail — Adaptive path

For each mathematical operation: equation, state, window/history, library
dependency, precision, determinism, and Go suitability.

| Operation | Equation (as coded) | State / window | Library | Deterministic? | Go suitable? |
|---|---|---|---|---|---|
| Reference & scale | `ref' = ref + α(price−ref)`; `scale' = max(min_scale, 0.95·scale + 0.05·|price−ref'|)` | 2 scalars, EWMA | stdlib | Yes | Yes, trivial |
| Level | `(price − ref)/max(scale, ε)` | prior ref/scale | stdlib | Yes | Yes |
| Velocity | `(level − prev_level)/(dt_eff + ε)`, clipped ±50 | 1 scalar | stdlib | Yes | Yes |
| Acceleration | `(vel − prev_vel)/(dt_eff + ε)`, clipped ±200 | 1 scalar | stdlib | Yes | Yes |
| Curvature | `accel/(1+vel²)^1.5`, clipped ±200 | none | stdlib (`**1.5`) | Yes | Yes; `math.Pow` |
| Innovation | `residual = level − (prev_level + prev_vel·dt)`; `mag = sqrt(residual²/max(dt+ε,ε))` | 2 scalars | `math.sqrt` | Yes | Yes |
| Volume influence | `ref' = (1−α)ref + α·vol`; `v* = log1p(vol/max(ref',ε)) + log1p(max(vol,0))/10`, bounded [0,3] | 1 scalar | `math.log1p` | Yes | Yes; `math.Log1p` |
| Coherence | `clamp01(|Σ wᵢxᵢ| / (Σ wᵢ|xᵢ| + ε))`, 0 if den ≤ ε | none; 4 channels | stdlib | Yes | Yes |
| Strength | `clamp(σ(bias + Σ cᵢ·featureᵢ − c_unc·unc), 0, 1)` | reads prior uncertainty | `math.exp` | Yes | Yes; `math.Exp` |
| Persistence | `(1−α)·prev + α·clamp01(dir_agree − 0.25·accel_penalty − pert_penalty)` | 1 scalar EWMA | stdlib | Yes | Yes |
| Uncertainty | `clamp(σ(Σ cᵢ·featureᵢ − 1), 0, 1)` | none | `math.exp` | Yes | Yes |
| Reversal propensity | `clamp(σ(Σ cᵢ·featureᵢ − 1.2), 0, 1)` | none | `math.exp` | Yes | Yes |
| Perturbation class | sign comparisons on residual, prior level, velocities vs 1e-15; quality floor 0.5; materiality floor `sqrt(ε)` | none | `math.sqrt` | Yes | Yes |
| Perturbation magnitude | `q = clamp01(innov/(1+innov))` | none | stdlib | Yes | Yes |
| Half-life adaptation | `hl' = clamp(hl · clamp(1+0.2·pers·str) · clamp(1−0.35·unc) · [0.75 if perturbed], 15, 900)` | 2 scalars | stdlib | Yes | Yes |
| Parameter adaptation | `η = clamp(η₀·max(0.2,1−unc)·max(0.5,str)·mult)`; `θ' = clamp(θ + η·0.1(str−unc))` | param dict | stdlib | Yes | Yes |
| Forward interval | `clamp(base·(0.7+0.6p)·(0.7+0.6s)·(1.1−0.8u)·(1−0.35m), 10, 600)` | none | stdlib | Yes | Yes |
| Forward samples | `τᵢ = L·(i/n)^1.8`, i=1..8 | none | `**` | Yes | Yes; `math.Pow` |
| Level propagation | `level + vel·τ + 0.5·accel·τ²` | none | stdlib | Yes | Yes |
| FMO decay | `2^(−τ/max(ε,fwd_hl))` | none | `**` | Yes | Yes; `math.Pow` |
| Terminal decay factor | `2^(−L/fwd_hl)` | none | `**` | Yes | Yes |
| D02 terminal displacement | `samples[−1].level − state_level` | 8 samples | stdlib | Yes | Yes |
| D02 max abs displacement | `max |sampleᵢ.level − state_level|` | 8 samples | stdlib | Yes | Yes |
| D04 geometry quality | `|terminal| / max_abs`, 0 if max_abs == 0 | none | stdlib | Yes | Yes |
| D04 structural quality | `(strength·coherence·persistence)^(1/3)` | none | `**(1/3)` | Yes | Yes; **see note** |
| D04 risk quality | `sqrt((1−unc)·(1−rev))` | none | `math.sqrt` | Yes | Yes |
| D04 capturability | `H · geometry · structural · risk` | none | stdlib | Yes | Yes |
| Rolling-15 adaptive props | median/min/max/range/last of C over 15; ΔC; up/down/flat counts; direction balance | **deque(maxlen=15)** | `statistics.median` | Yes | Yes; sort-based median |
| Decision predicate | `H==1 ∧ C ≥ median(C₁₅)` combined with direction agreement | rolling context | stdlib | Yes | Yes |
| State support ratio | `(strength·persistence)/max(ε, unc + rev)` | none | stdlib | Yes | Yes |

Notes on precision:

- `ε = 1e-8` (`NumericalConfig.epsilon`) is used both as a division guard and,
  via `sqrt(ε) = 1e-4`, as the perturbation materiality floor
  (`perturbation.py:68`, `model.py:339–342`). Go must reproduce `sqrt(1e-8)`
  identically — it will, since both use IEEE-754 `sqrt`, which is exactly
  rounded per the standard.
- `math.sqrt` is exactly rounded in both CPython and Go, so `sqrt` results are
  **bitwise identical**.
- `math.exp`, `math.log1p` and `math.pow` are *not* guaranteed exactly rounded.
  CPython delegates to the platform libm (MSVC on this machine); Go uses its own
  pure-Go implementations. Differences of 1–2 ulp are expected. Every sigmoid in
  D01 is followed by clamping to `[0,1]`, which does not remove the difference
  but does bound it. Equivalence must therefore be asserted at tolerance, not
  bitwise.
- `structural_quality` uses `x ** (1.0/3.0)`, not a true cube root. `1.0/3.0`
  is not exactly representable, so this is `pow(x, 0.333...33)`. A Go
  implementation must use `math.Pow(x, 1.0/3.0)` and **must not** substitute
  `math.Cbrt`, which would be more accurate and therefore *different*. This is a
  concrete example of the rule that no simplification may be introduced to make
  Go easier.
- `statistics.median` on 15 elements sorts and takes the middle element — no
  averaging, no floating-point arithmetic. A Go equivalent is exactly
  reproducible.

Determinism: the adaptive path has no randomness, no wall-clock dependency in
its mathematics, no iteration-order dependence over unordered containers that
affects numerical results, and no parallelism. `EnvelopeContext.production(evaluation_time=observation.event_time)`
(`emitter.py:215`) uses *source* time, not wall-clock, so evaluation is
reproducible. `time.perf_counter_ns` values are recorded but never feed
mathematics.

## C.3 Adaptive-path findings that affect scale

1. **Hot-path hashing.** Per observation, `canonical_sha256` runs three times
   over large nested dicts: `observation_id` over a small dict (line 225),
   `emission_id` over `emission_core` which embeds full `dmo.to_dict()`,
   `fmo.to_dict()` (8 samples × 7 fields) and `shape.to_dict()` (line 312), then
   `emission_hash` over the whole emission (line 314), then a *fourth* time at
   line 368 to verify non-mutation. Each call performs `json.dumps` with
   `sort_keys=True` over that structure. This is four full JSON serialisations
   and SHA-256 digests of a multi-hundred-field structure per observation, purely
   for identity and a self-check.
2. **Redundant recomputation.** `_adaptive_properties` is called up to three
   times per observation (lines 265, 316, 319) over the same 15-element context.
3. **`deepcopy` in the hot path.** Lines 262, 317, 362, 364 deepcopy context
   items and full emissions.
4. **Unbounded accumulation.** `emissions`, `initialization`,
   `adaptation_audit`, `feedback_audit` (emitter), `trace_records` (D01), `_rows`
   (pipeline) all grow forever. Observed: 331 adaptation + 170 feedback entries
   for 100 observations.
5. **pydantic model construction per observation.** `EnvelopeContext.production`
   and `CapturabilityResult` are pydantic models validated on every observation
   (`emitter.py:215`, `capturability_model.py:83`). In Go these become plain
   structs with explicit checks.

None of these are scientific requirements. Items 1–3 and 5 are pure overhead;
item 4 is a correctness problem at scale.

---

# Part D — D01 / D02 / D04 Analysis

## D.1 D01 — `sade/d01/v02/` (23 modules, 966 lines)

| Attribute | Finding |
|---|---|
| Purpose | Adaptive parametric state model. Converts a scalar price/volume observation stream into a normalised kinematic state vector plus derived quality channels, and emits a Deterministic Model Output (DMO) and an elastic Forward Model Output (FMO). |
| Inputs | `NormalizedObservation(entity_id, event_time, receive_time, sequence_id, price, volume, bid/ask, session, source_quality, availability_mask)` |
| Outputs | `DMOOutput` (26 fields), `FMOOutput` (`interval_length` + 8 `FMOSample`s) |
| Mathematical operations | 22-step sequence in `model.py::step`: causal validation → data quality → dt → reference/scale EWMA → kinematics → innovation → volume influence → perturbation classification → coherence → strength → persistence → uncertainty → reversal → half-life adaptation → bounded parameter adaptation → state commit → elastic forward interval + sample generation → health → DMO/FMO assembly → trace append |
| Persistent state | `RuntimeState`: `model_time, sequence, adaptive_reference, adaptive_scale, volume_reference, prev_level, prev_velocity, last_event_time, last_observation, parameter_state{ref_alpha}, parameter_update_magnitude, StateVector(10 floats), HalfLifeState(2 floats), clipping_count, nonfinite_count, parameter_bound_hits, innovation_extreme_count, data_gap_count` |
| Rolling state | None — D01 is pure single-step recursion. No windows, no history buffers. This is a significant finding: D01 is a **Markovian** update. |
| Adaptive behaviour | `update_parameters` adjusts `ref_alpha` by gradient `0.1·(strength − uncertainty)` with uncertainty/strength-scaled learning rate, bounded `[0.001, 0.2]`; `adapt_half_life` adjusts both half-lives multiplicatively with a 0.75 shortening on adverse perturbation |
| Feedback behaviour | Prior `uncertainty` feeds current `strength` (`model.py:154`); adapted `ref_alpha` feeds the next reference update (`model.py:236–237`); half-lives feed FMO decay |
| Numerical dependencies | `math.exp`, `math.sqrt`, `math.log1p`, `math.isfinite`, `**` |
| External Python packages | **NONE** — stdlib only across all 23 modules |
| Candidate Go equivalents | Go `math` package covers every function used. No third-party library needed. `RuntimeState` → a Go struct. `parameter_state map[string]float64` → struct fields or a small map. |
| Difficulty of Go migration | **LOW.** No arrays, no linear algebra, no history. Roughly 700 lines of Go for 966 lines of Python. |
| Scientific validation risk | **LOW–MEDIUM.** Risk is confined to transcendental-function last-bit differences (`exp`, `pow`) accumulating through the EWMA recursions across long streams. Because `adaptive_reference`, `adaptive_scale`, `persistence`, half-lives and `ref_alpha` are all recursive, a 1-ulp difference at step 1 propagates. Equivalence must be validated over long replays, not single steps. |

**Assignment: `GO_WITH_EQUIVALENCE_VALIDATION`.**

Evidence for the assignment: import scan over `sade/d01/` returns zero
non-stdlib imports; every function body read and confirmed scalar; `RuntimeState`
holds only scalars, two small dicts, and one prior observation.

Can D01 be implemented in Go without new mathematical design? **Yes.** Every
equation is closed-form and fully specified in code. No solver, no fitting, no
iteration to convergence, no library-specific algorithm choice.

## D.2 D02 — `sade/d02/v02/` (4 modules, 184 lines)

| Attribute | Finding |
|---|---|
| Purpose | Convert a `(DMOOutput, FMOOutput)` pair into a validated, immutable `ReturnShape` — the geometric summary of the projected path. |
| Inputs | `DMOOutput`, `FMOOutput` |
| Outputs | `ReturnShape` (17 fields incl. 8 `ForwardSample`s) |
| Mathematical operations | `terminal_displacement = samples[-1].level − state_level`; `maximum_absolute_displacement = max|sampleᵢ.level − state_level|`; `path_direction` = sign of terminal displacement (UPWARD/DOWNWARD/FLAT); `terminal_decay_factor = 2^(−interval/forward_half_life)` |
| Persistent state | **NONE.** Pure function. |
| Rolling state | None |
| Adaptive behaviour | None |
| Feedback behaviour | None |
| Numerical dependencies | `math.isfinite`, `**` |
| External Python packages | **NONE** — stdlib only |
| Candidate Go equivalents | Go structs + `math.Pow` + `math.IsInf/IsNaN` |
| Difficulty of Go migration | **VERY LOW.** Mostly validation. |
| Scientific validation risk | **LOW.** One `pow`; the rest is subtraction and comparison. |

Note: D02 is dominated by validation, not mathematics. `_validate_input` (44
lines) plus `ForwardSample.__post_init__` plus `ReturnShape.__post_init__`
re-check bounds that D01 already enforced — `projection_interval ∈ [10,600]`,
`forward_half_life ∈ [15,900]`, all quality channels `∈ [0,1]`, strictly
increasing `tau`, terminal `tau == interval_length`. These bounds are duplicated
from `ForwardConfig` and `HalfLifeConfig`. In Go this becomes a single explicit
validation function; the duplication should be preserved deliberately (defence in
depth) rather than silently dropped.

**Assignment: `GO_WITH_EQUIVALENCE_VALIDATION`** (arguably `GO_NOW`, but the one
`2^x` keeps it in the tolerance-checked category).

Can D02 be implemented in Go without new mathematical design? **Yes.**

## D.3 D04 — `sade/d04/` (7 modules, 239 lines)

| Attribute | Finding |
|---|---|
| Purpose | Capturability evaluation — score how capturable a `ReturnShape` is, under an `EnvelopeContext`. |
| Inputs | `ReturnShape`, `EnvelopeContext(context_role, provenance, evaluation_time, market_eligible)` |
| Outputs | `CapturabilityResult(hard_eligibility, geometry_quality, structural_quality, risk_quality, base_capturability_score, capturability_score, reason_codes)` |
| Mathematical operations | `Q_G = |terminal| / max_abs` (0 if max_abs==0); `Q_S = (strength·coherence·persistence)^(1/3)`; `Q_R = sqrt((1−uncertainty)·(1−reversal))`; `base = Q_G·Q_S·Q_R`; `H = int(projection_valid ∧ market_eligible ≠ False)`; `C = H · base` |
| Persistent state | **NONE.** `CapturabilityModelV0_2` is stateless; all methods static except `evaluate`. |
| Rolling state | None |
| Adaptive behaviour | None |
| Feedback behaviour | None. Note `EnvelopeState`, `CandidateStatus`, `SafetyState`, `EventType` enums exist in `enums.py` but the stateful `TradingEnvelope` runtime is explicitly excluded (`d04/__init__.py` docstring). 4 of 6 enums are unused in V0.1. |
| Numerical dependencies | `math.sqrt`, `**(1/3)` |
| External Python packages | **pydantic 2.13.4** — but only for `EnvelopeContext` and `CapturabilityResult`, i.e. field constraints (`Field(ge=0.0, le=1.0)`), `extra="forbid"`, `allow_inf_nan=False`, and a provenance cross-check validator. **No mathematics uses pydantic.** |
| Candidate Go equivalents | `math.Sqrt`, `math.Pow`; pydantic validation → explicit Go validation functions on plain structs |
| Difficulty of Go migration | **LOW** for mathematics. **LOW–MEDIUM** for the provenance validator, which encodes a real invariant: a `PRODUCTION` context may not carry `TEST_FIXTURE` provenance, a null field must be `UNAVAILABLE`, and a non-null field must not be `UNAVAILABLE` (`envelope_context.py:48–58`). That invariant is worth preserving explicitly in Go and is easy to lose in a naive port. |
| Scientific validation risk | **LOW.** One `sqrt` (exact), one `pow`. |

**Assignment: `GO_WITH_EQUIVALENCE_VALIDATION`.**

Can D04 be implemented in Go without new mathematical design? **Yes.**

An important behavioural detail worth flagging for human review, discovered by
reading `capturability_model.py:23–41`: `validate_return_shape` recomputes
`terminal` and `maximum` from the samples and requires **exact float equality**
with the values D02 already stored (`!=`, not a tolerance). This works today
because both are computed by identical Python expressions in the same process. A
Go implementation must compute them with the same operation order or this check
will fail. It is a hidden bitwise-equality coupling between D02 and D04.

## D.4 D01/D02/D04 summary

| Component | Modules | Lines | NumPy | SciPy | pydantic | State | Assignment | Difficulty |
|---|---:|---:|---|---|---|---|---|---|
| D01 | 23 | 966 | No | No | No | Markovian scalar | GO_WITH_EQUIVALENCE_VALIDATION | LOW |
| D02 | 4 | 184 | No | No | No | Stateless | GO_WITH_EQUIVALENCE_VALIDATION | VERY LOW |
| D04 | 7 | 239 | No | No | Validation only | Stateless | GO_WITH_EQUIVALENCE_VALIDATION | LOW |

None of D01, D02 or D04 provides any reason for Python to remain.

---

# Part E — Current Pricing Pipeline Forensics

## E.1 Verified actual flow

The task supplied an expected flow from prior migration evidence. Verified
against `sade/pricing_pipeline/pipeline.py::process`. Result: **the expected
flow is correct**, with four clarifications that matter.

```text
adaptive output record (dict)
   │  validate 8 required fields; entity match; strict source-order increment
   ▼
append to 8 parallel history lists; timestamp -> minutes (epoch/60)
   │
   ▼  index = len-1 ;  active_index = index - 1     <-- CLARIFICATION 1
causal_quadratic(times_minutes, closes, window=15)   -> p1[], p2[], failures
   │  np.linalg.lstsq on design [x², x, 1] over trailing 15, x recentred on x[index]
   │  requires rank == 3 and all coefficients finite
   │  p1 = coeff[1] ;  p2 = 2·coeff[0]
   ▼  if not finite at active_index -> WARMUP_DERIVATIVE
jp[i] = p2[i] − p2[i−1]  for all i                   <-- CLARIFICATION 2
   ▼  if active_index < 30 or jp window not all finite -> WARMUP_F4
fit_f4(p, p1, p2, jp, window=30, ridge_lambda=1.0)
   │  per index: values = [p, p1, p2] over trailing 30
   │  means, scales = mean/std (population, ddof=0)
   │  skip if any scale <= 0 or non-finite
   │  design = [1, (values−means)/scales]                (30×4)
   │  beta = solve(designᵀ·design + λ·diag(0,1,1,1), designᵀ·jp)   <-- ridge, intercept unpenalised
   │  slopes = beta[1:]/scales
   │  physical = [beta[0] − slopes·means, slopes]
   │  minimum/maximum = per-column min/max over the window  (domain envelope)
   │  condition = np.linalg.cond(design)
   ▼  if standardized not all finite at active_index -> F4_FIT_UNAVAILABLE
solve_cover([active_index], fit, p, p1, p2, time_term=False, rtol=1e-6, epsilon=0.00353...)
   │  initial state = [p, p1, p2] at active_index
   │  vector field: d/dt[p,p1,p2] = [p1, p2, jerk]
   │      jerk = beta[0] + Σ beta[1:4]·((state−means)/scales)     <-- CLARIFICATION 3
   │  atol = [rtol·scale_p, min(rtol·scale_p1, 0.1·epsilon), rtol·scale_p2]
   │  solve_ivp(method="RK45", t_span=(0,1), t_eval=linspace(0,1,11))
   │  reject if |trajectory[-1] − initial| > 1e6·scales  -> NUMERICALLY_UNSTABLE
   │  D_local_maximum = max ‖(traj − means)/scales‖₂
   │  envelope_exit if any traj point outside [minimum, maximum] per component
   ▼
build_numerical_row(...)
   │  matrix = [[0,1,0],[0,0,1], physical[1:4]]        <-- companion form of local dynamics
   │  eigenvalues  = np.linalg.eigvals(matrix)          -> max_real_eigenvalue
   │  amplification = max over columns of ‖expm(matrix)[:,j]‖₂   (scipy.linalg.expm)
   │  projected = trajectory[-1]  (or current state if RK failed)
   ▼
PriceEngine.observe(observation, numerical, policy_state)
   │  assert numerical.symbol == observation.symbol and timestamps match
   ▼
EmissionPolicy.emit
   │  if not rk_success or non-finite -> INVALID emission, state reset
   │  phase        <- (p1, p2, projected_p1, epsilon)
   │  tendency     <- (p1, p2, projected_p1, projected_p2, epsilon)
   │  domain       <- domain_exit
   │  confidence   <- (condition, eigenvalue, amplification) vs median / q95 thresholds
   │  stability    <- eigenvalue <= 0 ? STABLE : LOCALLY_EXPANSIVE
   │  raw_color    <- phase + projected_p1 sign
   │  color        <- raw_color with GREEN<->RED direct-reversal debounce
   ▼  PriceEmission (29 fields) + next PolicyState
PriceCockpitInterpreter.observe
   │  motion_state, p1_zero_proximity, deceleration_strength
   │  opposing-direction persistence counter, turn candidate + hysteresis
   │  cockpit_color
   ▼  PriceCockpitEmission (16 fields) + next CockpitState   <-- CLARIFICATION 4
```

**Clarification 1 — the pricing stage runs one observation behind.**
`active_index = index − 1` (`pipeline.py:268`). Every pricing output is for the
*previous* observation, because `jp` needs `p2` at both `i` and `i−1`. This is a
structural one-observation latency inherent to the current mathematics, not an
implementation artifact. Any latency budget must account for it.

**Clarification 2 — `p` is close price, and `jp` is the target, not a state
variable.** `fit_f4` regresses `jp` (the discrete difference of `p2`) on the
standardised `[p, p1, p2]` state. So the "F4" model is a local affine model of
jerk as a function of position/velocity/acceleration.

**Clarification 3 — `time_term` is hard-wired off.** `pipeline.py:299` passes
`False`. The `beta[:,4]` time-term branch (`projection.py:93–94`) and the
`time_mean`/`time_scale` fit keys are therefore dead in production — and indeed
`allocate_fit` never allocates `time_mean`/`time_scale`, so enabling
`time_term` would raise `KeyError`. Dead-but-broken branch.

**Clarification 4 — cockpit is downstream of PriceEmission and is enabled by
default.** `PricingPipelineConfig.enable_cockpit = True`, and the validated run
produced 55 cockpit outputs for 55 emissions.

## E.2 Verified against the recorded run

`output/unit_runs/pricing_001/pricing_summary.json` corroborates the traced
control flow exactly:

| Counter | Value | Consistent with |
|---|---:|---|
| `observations_received` | 100 | 100-vector stream |
| `WARMUP_DERIVATIVE` | 15 | derivative window 15 → first 14 indices lack a fit, plus `active_index<0` at index 0 |
| `WARMUP_F4` | 30 | F4 window 30 |
| `derivative_ready_observations` | 85 | 100 − 15 |
| `f4_ready_observations` | 55 | 85 − 30 |
| `rk45_attempts` / `successes` / `failures` | 55 / 55 / 0 | one solve per F4-ready observation; **zero RK45 failures** |
| `domain_exits` | 18 | 33% of solves left the local envelope |
| `price_emissions_generated` | 55 | one per attempt |
| `price_cockpit_outputs` | 55 | cockpit enabled |
| `confidence_state_counts` | LOW 26, MEDIUM 29, HIGH 0 | **no HIGH-confidence emission occurred** |
| `price_color_counts` | AMBER 33, GREEN 14, RED 8 | 55 total |

Two observations for human review. First, RK45 never failed in the validated
run — 55/55 success, which weakens the "RK45 is numerically fragile so keep
SciPy" argument on *robustness* grounds, though not on *implementation-effort*
grounds. Second, zero HIGH-confidence emissions were produced, and 26 of 55 were
LOW. Since confidence is driven by `condition_number`, `max_real_eigenvalue` and
`perturbation_amplification` against fixed thresholds, this is precisely the
classification surface most exposed to numerical drift during migration.

---

# Part F — Pricing Mathematics Go Refactorability

## F.1 Go capability inventory (from local evidence only)

Verified by inspecting `C:\Users\chino\go\pkg\mod`:

- `gonum.org/v1/gonum` **v0.16.0 and v0.17.0 are both fully extracted** in the
  local module cache (3,972 filesystem entries under `gonum.org`), including the
  `mat`, `stat`, `floats`, `lapack`, `blas`, `optimize`, `interp`, `mathext`,
  `diff`, `num` and `integrate` packages.
- `gonum.org/v1/gonum v0.17.0` is hash-pinned in `SDX/go.sum` line 31–32 and
  appears in the module graph via `go list -m all`.
- It is **not** declared in `SDX/go.mod` require blocks, and `go mod why`
  reports "main module does not need package gonum.org/v1/gonum". No SDX file
  imports it.

Conclusion: adopting gonum requires a `go.mod` declaration change but **no
network access**. This is recorded as ALREADY PRESENT (resolvable offline, not
yet declared).

Confirmed gonum API surface relevant to this migration (symbols verified in
`gonum@v0.17.0/mat`):

| Need | gonum symbol | Present |
|---|---|---|
| Dense matrix | `mat.NewDense` | Yes |
| Linear solve | `(*mat.Dense).Solve` | Yes |
| Matrix inverse | `(*mat.Dense).Inverse` | Yes |
| Condition number | `mat.Cond` | Yes |
| Eigenvalues (general) | `(*mat.Eigen).Factorize` | Yes |
| Eigenvalues (symmetric) | `(*mat.EigenSym).Factorize` | Yes |
| **Matrix exponential** | `(*mat.Dense).Exp` | **Yes** |
| Matrix power | `(*mat.Dense).Pow` | Yes |
| QR factorisation | `(*mat.QR).Factorize` | Yes |
| SVD | `(*mat.SVD).Factorize` | Yes |
| LU | `(*mat.LU).Factorize` | Yes |
| Cholesky | `(*mat.Cholesky).Factorize` | Yes |
| Mean / std / quantile | `stat` package | Yes |
| Float slice ops | `floats` package | Yes |
| **IVP / ODE solver (RK45)** | — | **NO** |

The `integrate` package contains only `quad` and `testquad` — quadrature, not
initial-value integration. A recursive directory search for package names
matching `ode|ivp|runge|rk` across the extracted tree returned no numerical
integration package. **There is no adaptive Runge–Kutta implementation anywhere
in the local Go dependency universe.**

`(*mat.Dense).Exp` is documented in source as implementing "Functions of
Matrices: Theory and Computation, Chapter 10, Algorithm 10.20" (Higham), using
scaling-and-squaring with Padé approximants at thresholds θ ∈ {0.015, 0.25,
0.95, 2.1, …}. `scipy.linalg.expm` uses the same Higham family. The algorithms
are the same *class* with potentially different θ tables and squaring counts —
so results will agree to near machine precision but are **not guaranteed
bitwise identical**. That is exactly why `perturbation_amplification` needs
equivalence validation rather than assumption.

Telemetry and infrastructure already available in the Go module graph:
`go.opentelemetry.io/otel` 1.39.0 with `sdk`, `sdk/metric`, `metric` and `trace`
(pulled in via grpc), `google.golang.org/grpc` 1.80.0, `google.golang.org/protobuf`
1.36.11, `github.com/google/uuid` 1.6.0, `golang.org/x/sync` 0.19.0
(`errgroup`, `semaphore`). Channels, `select`, `sync`, `context` and goroutines
are language/stdlib features requiring nothing.

Capability classification:

**ALREADY PRESENT (in module graph / cache, no network needed)**
matrices; general and symmetric eigenvalues; condition numbers; linear solve;
QR/SVD/LU/Cholesky least squares; matrix exponential; matrix power; mean/std/
quantile statistics; float slice utilities; goroutines; typed channels; `select`;
`context`; `sync` primitives; `errgroup`/`semaphore`; gRPC; protobuf;
OpenTelemetry traces and metrics; UUID generation.

**WOULD REQUIRE NEW DEPENDENCY**
Nothing identified as strictly necessary. `gonum` must be *declared* in
`go.mod`, but is already cached and hash-pinned. Azure SDK packages would be new,
but are out of scope for local work.

**WOULD REQUIRE CUSTOM IMPLEMENTATION**
1. **Adaptive RK45 (Dormand–Prince) initial-value solver** with per-component
   absolute tolerance vector, dense output at prescribed `t_eval` points, step
   acceptance/rejection, and `nfev`/status reporting. This is the single genuine
   gap. Estimated 250–400 lines of Go plus a substantial test suite.
2. A local Pub/Sub broker abstraction (trivial over channels; ~150 lines).
3. A partition-owner registry / lifecycle supervisor (~200–300 lines).
4. Canonical deterministic hashing equivalent to
   `json.dumps(sort_keys=True, separators=(",",":"))` + SHA-256, **only if
   `observation_id`/`emission_id` values must remain byte-identical to the Python
   baseline.** Go's `encoding/json` orders struct fields by declaration and map
   keys lexicographically, and formats floats differently from CPython's `repr`.
   Reproducing CPython's float repr exactly is genuinely fiddly. See Part L risk.

## F.2 Per-function pricing mathematics assessment

### F.2.1 `causal_quadratic` — causal quadratic fitting (p1 / p2)

- **CURRENT PYTHON LIBRARY:** NumPy — `np.column_stack`, `np.linalg.lstsq(design, y, rcond=None)`, `np.all(np.isfinite(...))`, `np.full`
- **CURRENT FUNCTION:** `sade/pricing_pipeline/derivatives.py:44–95`
- **MATHEMATICAL PURPOSE:** Fit `y ≈ a·x² + b·x + c` by least squares over the trailing 15 observations, with `x` recentred so `x = 0` at the current index. Then `p1 = b` (first derivative at the current point) and `p2 = 2a` (second derivative). Recentring is what makes it causal and evaluated *at* the endpoint.
- **GO STANDARD LIBRARY SUFFICIENT?** No — needs least squares on a 15×3 system.
- **MATURE GO LIBRARY AVAILABLE?** Yes — `mat.QR.Factorize` + `SolveTo`, or `mat.SVD` for a rank-revealing equivalent. Both present in cache.
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No, but the `rank != 3` guard needs care: `np.linalg.lstsq` with `rcond=None` uses SVD (LAPACK `gelsd`) and derives rank from singular values against a machine-epsilon-scaled threshold. To reproduce the guard faithfully, use `mat.SVD` and apply the same rank criterion. Using QR without a rank check would change failure behaviour.
- **STATEFUL?** No — pure function over supplied arrays.
- **NUMERICAL RISK:** **MEDIUM.** Different least-squares algorithms (SVD vs QR) give results differing at ~1e-14 relative on a well-conditioned 15×3 system. But `p2` is multiplied by 2 and then differenced to form `jp`, and differencing amplifies relative error. `jp` is then the regression *target* for F4.
- **MIGRATION COMPLEXITY:** LOW-MEDIUM.
- **RECOMMENDATION:** `GO_WITH_EQUIVALENCE_VALIDATION`. Use `mat.SVD` to mirror `gelsd`. Validate `p1`, `p2`, and the `failures` count on fixed input vectors, and validate `jp` explicitly since it is the error-amplifying quantity.

### F.2.2 `p` / `p1` / `p2` calculation and derivative-state logic

- **CURRENT PYTHON LIBRARY:** `p` is just `closes` as a NumPy array (`pipeline.py:273`). `derivative_state` uses stdlib `math` only.
- **CURRENT FUNCTION:** `pipeline.py:273–279`; `derivatives.py:98–131`
- **MATHEMATICAL PURPOSE:** `p` = close price. `derivative_state` maps `(p1, p2)` into 8 categorical labels (RISING_STRENGTHENING, UPPER_TURNING_REGION, etc.).
- **GO STANDARD LIBRARY SUFFICIENT?** Yes.
- **MATURE GO LIBRARY AVAILABLE?** N/A.
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No.
- **STATEFUL?** No.
- **NUMERICAL RISK:** NONE (comparisons only).
- **MIGRATION COMPLEXITY:** TRIVIAL.
- **RECOMMENDATION:** `GO_NOW`. Note `derivative_state` is **not called in production** — confirm with a human whether it should be carried forward at all.

### F.2.3 `fit_f4` — F4 fitting, standardisation, ridge, matrix ops, condition number

- **CURRENT PYTHON LIBRARY:** NumPy — `mean(axis=0)`, `std(axis=0)`, `column_stack`, `diag`, `@`, `np.linalg.solve`, `np.linalg.cond`, `np.r_`, `min/max(axis=0)`
- **CURRENT FUNCTION:** `sade/pricing_pipeline/dynamics.py:55–111`
- **MATHEMATICAL PURPOSE:** Standardise `[p, p1, p2]` over a trailing 30-window; build design `[1, z]`; solve ridge normal equations `(XᵀX + λ·diag(0,1,1,1))β = Xᵀ·jp` — intercept deliberately unpenalised; de-standardise to physical coefficients; record the per-component window min/max as the local validity envelope; record `cond(design)`.
- **GO STANDARD LIBRARY SUFFICIENT?** No.
- **MATURE GO LIBRARY AVAILABLE?** Yes — `mat.Dense.Mul`, `mat.Dense.Solve` (4×4), `mat.Cond` (30×4), `stat.Mean`/`stat.StdDev` (**caution:** `stat.StdDev` is the *sample* standard deviation with Bessel correction; `np.std` defaults to `ddof=0`, the *population* standard deviation. These differ by a factor of `sqrt(n/(n-1))` = `sqrt(30/29)` ≈ 1.0174. Using `stat.StdDev` naively would silently change every scale, every standardised value, and every coefficient. This is the single most dangerous line-for-line translation trap in the whole migration.)
- **CUSTOM GO IMPLEMENTATION REQUIRED?** Partially — population std must be written explicitly or `stat.StdDev` corrected. `mat.Cond` must be called with the same norm: `np.linalg.cond` with default arguments uses the **2-norm** (ratio of largest to smallest singular value); `mat.Cond(a, norm)` requires an explicit norm and must be given `2`.
- **STATEFUL?** No — pure function, but currently recomputes all indices.
- **NUMERICAL RISK:** **HIGH.** Three compounding reasons: (a) forming `XᵀX` squares the condition number, so the solve is genuinely sensitive to the LAPACK path (`np.linalg.solve` uses LU with partial pivoting via `gesv`; `mat.Dense.Solve` chooses LU for square systems, so these should align, but pivot selection on near-ties can differ); (b) `ddof` and norm mismatches are silent and catastrophic; (c) `cond` feeds discrete confidence thresholds in `EmissionPolicy.emit`, so a small numerical difference can flip a categorical output.
- **MIGRATION COMPLEXITY:** MEDIUM.
- **RECOMMENDATION:** `GO_WITH_EQUIVALENCE_VALIDATION` with the strictest tolerance budget of any function in the system. Validate `means`, `scales`, `standardized`, `physical`, `minimum`, `maximum`, and `condition` independently, then validate that the *derived confidence label* matches across a long replay — because label agreement, not float agreement, is the real requirement.

### F.2.4 `solve_cover` — RK45 one-step projection and domain-exit logic

Treated in full in Part G (RK45 Special Investigation).

- **CURRENT PYTHON LIBRARY:** SciPy `scipy.integrate.solve_ivp`; NumPy for `linspace`, `column_stack`, `reshape`, `transpose`, `linalg.norm`, `flatnonzero`, boolean masking
- **CURRENT FUNCTION:** `sade/pricing_pipeline/projection.py:42–157`
- **MATHEMATICAL PURPOSE:** Integrate `d/dt[p,p1,p2] = [p1, p2, J(p,p1,p2)]` from the current state over `t ∈ [0,1]` minute, where `J` is the F4 local affine jerk model; evaluate at 11 grid points; detect exit from the local `[minimum, maximum]` envelope; compute the maximum standardised local distance.
- **GO STANDARD LIBRARY SUFFICIENT?** No.
- **MATURE GO LIBRARY AVAILABLE?** **NO.**
- **CUSTOM GO IMPLEMENTATION REQUIRED?** **YES** — an adaptive Dormand–Prince RK45.
- **STATEFUL?** No.
- **NUMERICAL RISK:** MEDIUM-HIGH for the trajectory; but note only `trajectory[-1]` reaches `PriceEmission` as `projected_p/p1/p2`, plus the boolean `envelope_exit`, `first_exit_time`, `exit_dimension` and `D_local_maximum`. The intermediate grid points affect only envelope-exit detection.
- **MIGRATION COMPLEXITY:** **HIGH** — the only high-complexity item in the system.
- **RECOMMENDATION:** `RETAIN_PYTHON_INITIAL_V0_1`, then `GO_LATER`.

### F.2.5 Eigenvalues, perturbation amplification, matrix exponential

- **CURRENT PYTHON LIBRARY:** `np.linalg.eigvals`, `scipy.linalg.expm`, `np.linalg.norm`
- **CURRENT FUNCTION:** `sade/pricing_pipeline/numerical.py:86–89`
- **MATHEMATICAL PURPOSE:** Build the companion matrix `A = [[0,1,0],[0,0,1],[c₁,c₂,c₃]]` from the physical F4 coefficients; `max_real_eigenvalue = max Re(λ(A))` (local stability); `perturbation_amplification = max_j ‖expm(A)[:,j]‖₂` — the largest column norm of the one-minute state-transition matrix, i.e. worst-case growth of a unit perturbation over the projection horizon.
- **GO STANDARD LIBRARY SUFFICIENT?** No.
- **MATURE GO LIBRARY AVAILABLE?** **Yes** — `mat.Eigen.Factorize` (with `mat.EigenRight` or values-only) and `(*mat.Dense).Exp`, both present in cache. `floats.Norm` for the column 2-norm.
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No.
- **STATEFUL?** No.
- **NUMERICAL RISK:** **MEDIUM-HIGH,** for the same discrete-threshold reason as `cond`: both feed `EmissionPolicy` confidence classification, and `max_real_eigenvalue ≤ 0` directly determines `stability_state` (`policy.py:204`). A sign flip near zero flips STABLE ↔ LOCALLY_EXPANSIVE. Also, `np.linalg.eigvals` returns eigenvalues in LAPACK order; taking `.real.max()` is order-independent, which helps. `expm` algorithm-table differences are the main exposure.
- **MIGRATION COMPLEXITY:** LOW-MEDIUM (library calls), MEDIUM to *validate*.
- **RECOMMENDATION:** `GO_WITH_EQUIVALENCE_VALIDATION`. Validate `max_real_eigenvalue` and `perturbation_amplification` against SciPy on a wide sweep of companion matrices, and specifically probe eigenvalues near zero and condition numbers near the policy thresholds.

### F.2.6 Domain-exit logic

- **CURRENT PYTHON LIBRARY:** NumPy boolean masking / `flatnonzero`
- **CURRENT FUNCTION:** `projection.py:124–137`
- **MATHEMATICAL PURPOSE:** Per grid point, test all three components against `[minimum, maximum]`; `inside = all components inside`; first exit index → `first_exit_time`; exit components joined as `"P|P1|P2"`.
- **GO STANDARD LIBRARY SUFFICIENT?** Yes (loops + `strings.Join`).
- **MATURE GO LIBRARY AVAILABLE?** N/A.
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No.
- **STATEFUL?** No.
- **NUMERICAL RISK:** NONE beyond trajectory inputs.
- **MIGRATION COMPLEXITY:** TRIVIAL.
- **RECOMMENDATION:** `GO_NOW` (moves with the RK45 caller).

### F.2.7 Price policy — `EmissionPolicy`

- **CURRENT PYTHON LIBRARY:** stdlib `math.isfinite` only
- **CURRENT FUNCTION:** `sade/pricing_pipeline/price_engine/policy.py` (252 lines)
- **MATHEMATICAL PURPOSE:** Categorical classification: `_direction`, `_acceleration`, `_phase`, `_turning_tendency`, confidence tiering, stability, colour, GREEN↔RED direct-reversal debounce.
- **GO STANDARD LIBRARY SUFFICIENT?** **Yes, entirely.**
- **MATURE GO LIBRARY AVAILABLE?** Not needed.
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No.
- **STATEFUL?** `PolicyState(previous_color, pending_reversal)` — but externally owned and returned, i.e. already a **pure state-transition function**. This is the ideal shape for a Go partition-owner goroutine.
- **NUMERICAL RISK:** NONE — no arithmetic beyond subtraction for deltas.
- **MIGRATION COMPLEXITY:** LOW.
- **RECOMMENDATION:** `GO_NOW`. One caution: `reason_codes=tuple(dict.fromkeys(reasons))` relies on Python dict insertion-order preservation to deduplicate while keeping order. Go maps do **not** preserve order; an ordered dedupe over a slice is required or `reason_codes` will differ.

### F.2.8 Price state

- **CURRENT FUNCTION:** `PolicyState` (policy.py:85), `CockpitState` (cockpit.py:58)
- **MATHEMATICAL PURPOSE:** Carry minimal cross-observation memory: previous colour, pending reversal, previous motion state, opposing direction + count, candidate direction + age.
- **GO STANDARD LIBRARY SUFFICIENT?** Yes — frozen dataclasses → immutable Go structs returned by value.
- **STATEFUL?** Yes, by definition — but explicitly threaded, never hidden.
- **NUMERICAL RISK:** NONE.
- **MIGRATION COMPLEXITY:** TRIVIAL.
- **RECOMMENDATION:** `GO_NOW`. These two structs are the entire per-channel pricing state and total 8 fields.

### F.2.9 Cockpit logic

- **CURRENT PYTHON LIBRARY:** stdlib `math.isfinite`, `math.nan`
- **CURRENT FUNCTION:** `sade/pricing_pipeline/price_engine/cockpit.py` (248 lines)
- **MATHEMATICAL PURPOSE:** `zero_proximity = |projected_p1| / max(|p1|, |projected_p1|, ε)`; `deceleration_strength = opposing_change / max(|p1|, ε)`; opposing-direction persistence counting; zero-crossing turn candidates; candidate hysteresis; refined internal state and cockpit colour.
- **GO STANDARD LIBRARY SUFFICIENT?** **Yes, entirely.**
- **CUSTOM GO IMPLEMENTATION REQUIRED?** No.
- **STATEFUL?** `CockpitState`, externally owned — same pure-transition shape as `PolicyState`.
- **NUMERICAL RISK:** **LOW.** Two divisions with explicit guards. No transcendentals.
- **MIGRATION COMPLEXITY:** LOW.
- **RECOMMENDATION:** `GO_NOW`. Same `dict.fromkeys` ordered-dedupe caution.

### F.2.10 PriceEngine

- **CURRENT FUNCTION:** `price_engine/engine.py::PriceEngine.observe` (39 executable lines)
- **MATHEMATICAL PURPOSE:** None. It validates that `numerical["symbol"]` equals `observation.symbol` and timestamps match, then delegates to the policy.
- **RECOMMENDATION:** `GO_NOW`. This is a coherence gate, not mathematics.

## F.3 Pricing mathematics summary

| Function | Library today | Go path | Numerical risk | Recommendation |
|---|---|---|---|---|
| `causal_quadratic` | `np.linalg.lstsq` | `mat.SVD` | MEDIUM | GO_WITH_EQUIVALENCE_VALIDATION |
| `derivative_state` | stdlib | stdlib | NONE | GO_NOW (unused today) |
| `fit_f4` | `np.linalg.solve`, `np.linalg.cond`, `np.std` | `mat.Dense.Solve`, `mat.Cond(·,2)`, explicit population std | **HIGH** | GO_WITH_EQUIVALENCE_VALIDATION |
| `valid_fit` | `np.isfinite` | `math.IsNaN/IsInf` | NONE | GO_NOW |
| `solve_cover` (RK45) | `scipy.integrate.solve_ivp` | **none — custom Dormand–Prince** | MEDIUM-HIGH | RETAIN_PYTHON_INITIAL_V0_1 → GO_LATER |
| domain-exit logic | NumPy masking | loops | NONE | GO_NOW |
| `eigvals` | `np.linalg.eigvals` | `mat.Eigen` | MEDIUM-HIGH | GO_WITH_EQUIVALENCE_VALIDATION |
| `expm` amplification | `scipy.linalg.expm` | `mat.Dense.Exp` | MEDIUM-HIGH | GO_WITH_EQUIVALENCE_VALIDATION |
| `build_numerical_row` assembly | NumPy + `json.dumps` | structs + `encoding/json` | LOW | GO_NOW (except the two above) |
| `EmissionPolicy` | stdlib | stdlib | NONE | GO_NOW |
| `PriceCockpitInterpreter` | stdlib | stdlib | LOW | GO_NOW |
| `PriceEngine` | stdlib | stdlib | NONE | GO_NOW |
| `PolicyState` / `CockpitState` | dataclasses | structs | NONE | GO_NOW |

Of 822 lines in `price_engine/`, **all 822 are stdlib-only and carry no
numerical migration risk.** The entire Price Engine, policy and cockpit can move
to Go with no library dependency at all. Of 898 lines in `pricing_pipeline/`,
only **146** (`projection.py`) lack a Go path.

---

# Part G — RK45 Special Investigation

The task instructed that RK45 must not be assumed to be the reason Python
remains. It was investigated from code.

## G.1 Current Python implementation

`sade/pricing_pipeline/projection.py::solve_cover`, 146 lines.

**SciPy usage:** exactly one import — `from scipy.integrate import solve_ivp`
(line 39). No other SciPy symbol is used in this file.

**`solve_ivp` configuration** (lines 106–114):

| Parameter | Value | Source |
|---|---|---|
| `fun` | closure `function(time, flattened)` | lines 89–95 |
| `t_span` | `(0.0, 1.0)` — fixed one-minute horizon | line 108 |
| `y0` | `initial.ravel()` = `[p, p1, p2]` per index, flattened | line 109 |
| `method` | `"RK45"` (Dormand–Prince 5(4)) | line 110 |
| `rtol` | `1e-6` from `PricingPipelineConfig.rtol` | line 111 |
| `atol` | **per-component vector**: `[rtol·scale_p, min(rtol·scale_p1, 0.1·epsilon), rtol·scale_p2]` | lines 97–103 |
| `t_eval` | `np.linspace(0.0, 1.0, 11)` — 11 dense output points | line 113 |

**Tolerances:** `rtol = 1e-6`. `atol` is not scalar — it is scaled by the
per-component standardisation scales from the F4 fit, with the velocity
component additionally capped at `0.1 · epsilon` where
`epsilon = 0.0035332071428566536`, giving `atol[1] ≤ 3.533e-4`. Reproducing this
per-component atol vector is mandatory; a scalar-atol Go solver would not be
equivalent.

**State dimension:** 3 per observation (`p`, `p1`, `p2`). The code is written to
batch multiple observations by flattening into `3·k` and reshaping with
`state_values = flattened.reshape(-1, 3)` — but **production always passes a
single index**: `pipeline.py:299` calls `solve_cover([active_index], ...)`. So in
production the state dimension is exactly **3**, and the vectorised batching, the
1024-chunk loop (line 155) and the recursive bisection-on-failure
(lines 148–153) are all dormant single-element paths.

**Integration horizon:** `[0, 1]` in minutes. Explicitly documented as distinct
from source timestamp spacing (`projection.py` docstring line 24) — verified
correct: the validated run saw source gaps of 60–300 s
(`summary.json`: `source_delta_t_seconds_min: 60.0`, `max: 300.0`), while the
projection horizon stayed fixed at 1 minute.

**Vector field:** `d/dt[p, p1, p2] = [p1, p2, J]` where
`J = β₀ + Σ_{i=1..3} βᵢ · ((stateᵢ − meanᵢ)/scaleᵢ)`. This is a **linear
(affine) ODE with constant coefficients** — the companion matrix built in
`numerical.py:87` is exactly its Jacobian, which is why the matrix exponential
is meaningful there.

**This is the most important finding of the RK45 investigation.** The system
being integrated is affine with constant coefficients over the horizon. Its
exact closed-form solution is `y(t) = expm(A·t)·(y₀ − y*) + y*`, and the code
*already computes `expm(A)`* in `numerical.py:89`. The system is not stiff, not
nonlinear, not discontinuous, and not time-dependent (`time_term=False`). An
adaptive Runge–Kutta solver is being used on a problem that has an analytic
solution the codebase already has the machinery to evaluate.

This does **not** license replacing RK45 with the closed form — that would be a
mathematical simplification, which §47 of the task forbids, and the numerical
outputs would differ. But it is decisive for *risk assessment*: a Go
Dormand–Prince implementation faces the easiest possible test problem. It is not
being asked to handle stiffness or discontinuity. Equivalence to ~1e-12 relative
is a realistic target, and the analytic solution provides an *independent third
reference* for validating both implementations.

**Failure behaviour:**
- `not solution.success or not all finite` → `raise RuntimeError(solution.message)`
  (lines 115–116), caught at line 147.
- Explicit instability rejection: `|trajectory[-1] − initial| > 1e6 · scales`
  → `failed[obs] = "NUMERICALLY_UNSTABLE"` (lines 120–122).
- On exception with a single index: `failed[group[0]] = str(error)`;
  with multiple: recursive bisection (dormant in production).
- Downstream, `build_numerical_row` treats a failed index by substituting the
  *current* state as `projected` with `rk_success=False`, `domain_exit=False`
  (`numerical.py:91–94`), and `EmissionPolicy.emit` then emits an INVALID
  emission with reason `RK_FAILURE` and resets `PolicyState`
  (`policy.py:174–177`).

**Domain behaviour:** the solver is unconstrained; the `[minimum, maximum]`
envelope is checked *after* integration, and exit is reported rather than
enforced. So domain exit is diagnostic, not a solver constraint.

**Expected invocation frequency:** one `solve_ivp` call per F4-ready
observation. Measured: **55 calls per 100 observations** (55%) in the validated
run. Extrapolating architecturally: at 1,000 channels each producing one
observation per minute, ~550 calls/min ≈ 9/s. At one observation per second per
channel, ~550 calls/s.

**Expected computational cost:** NOT YET MEASURED. No timing instrumentation
exists for `solve_cover`. What *is* known from code: a 3-dimensional non-stiff
affine system integrated over one unit of time at `rtol=1e-6` with 11 dense
output points will take on the order of tens of RHS evaluations. The dominant
cost is therefore not the arithmetic but the **Python/SciPy call overhead**:
`solve_ivp` setup, the Python-level RHS closure invoked per stage (each
invocation doing NumPy reshape, broadcast subtract/divide, and `column_stack`
on 1×3 arrays — where NumPy overhead vastly exceeds the arithmetic), and dense
output interpolation. This is a strong indication that a Go implementation would
be *faster*, not slower, since it removes both the interpreter and the
per-call NumPy overhead on tiny arrays.

## G.2 Does an equivalent Go implementation already exist locally?

**No.** Verified by inspecting the extracted local Go module cache:

- `gonum@v0.17.0/integrate/` contains only `quad` and `testquad` — Gaussian and
  related quadrature rules for definite integrals, not initial-value problems.
- A recursive search of the entire extracted `gonum.org` tree for directories
  matching `ode|ivp|runge|rk` returned only `.github/workflows` and
  `graph/network` — no numerical integration package.
- No other module in the graph (grpc, protobuf, otel, golang.org/x/*) provides
  ODE integration.

No web research was performed, per instruction. The conclusion is therefore
scoped precisely: **no adaptive RK45 exists in the current local Go dependency
set.** It would have to be written.

## G.3 RK45 classification

**`RETAIN_PYTHON_INITIAL_V0_1`**, with an explicit `GO_LATER` commitment.

Evidence for retaining initially:
1. It is the only mathematical operation in the entire system with no available
   Go implementation (Part F.1).
2. Faithful reproduction requires the adaptive step controller, the
   per-component `atol` vector, dense output at `t_eval`, and `nfev`/`message`
   reporting — new numerical infrastructure, not a library call.
3. It is a small, well-isolated surface: one function, 146 lines, one entry
   point, pure (no state), with a simple in/out contract.

Evidence *against* treating it as permanently Python:
1. The ODE is affine, constant-coefficient, non-stiff, 3-dimensional and
   integrated over a single unit interval — the easiest class of IVP.
2. It never failed in the validated run (55/55 success).
3. An analytic reference solution exists via `expm(A·t)`, giving an independent
   validation oracle that most migrations do not have.
4. If retained in Python behind a cross-process boundary, it becomes the
   throughput bottleneck of the entire runtime at ~55% of observations
   (Part J).
5. The production call is always single-index, so the Go implementation needs to
   handle a 3-dimensional system only — the batching machinery need not be
   ported.

**Is RK45 the primary reason Python remains? Yes — and it is the *only* reason.**
Every other Python dependency in the system either has a gonum equivalent
already in the local cache, or is stdlib-only.

---

# Part H — Python Library Dependency Matrix and Boundary

## H.1 Dependency matrix — production-relevant SADE Python

| Module | Function/class | numpy | scipy | sklearn | pandas | stdlib only | Other |
|---|---|---:|---:|---:|---:|---:|---|
| `adaptive_pipeline/pipeline.py` | all | – | – | – | – | ✔ | — |
| `adaptive_emitter/emitter.py` | all | – | – | – | – | ✔ | — |
| `adaptive_emitter/normalizer.py` | all | – | – | – | – | ✔ | — |
| `configuration/scientific_baseline.py` | all | – | – | – | – | ✔ | — |
| `input/sdx_client.py` | all | – | – | – | – | – | grpcio |
| `input/generated/sdx/v1/*` | generated | – | – | – | – | – | grpcio, protobuf |
| `d01/v02/adaptation.py` | `update_parameters` | – | – | – | – | ✔ | — |
| `d01/v02/coherence.py` | `compute_coherence` | – | – | – | – | ✔ | — |
| `d01/v02/config.py` | 15 dataclasses | – | – | – | – | ✔ | — |
| `d01/v02/forward.py` | 3 functions | – | – | – | – | ✔ | — |
| `d01/v02/half_life.py` | `adapt_half_life` | – | – | – | – | ✔ | — |
| `d01/v02/health.py` | `evaluate_health` | – | – | – | – | ✔ | — |
| `d01/v02/innovation.py` | `innovation_magnitude` | – | – | – | – | ✔ | — |
| `d01/v02/kinematics.py` | `compute_kinematics` | – | – | – | – | ✔ | — |
| `d01/v02/model.py` | `D01V02Model.step` | – | – | – | – | ✔ | — |
| `d01/v02/observations.py` | all | – | – | – | – | ✔ | — |
| `d01/v02/outputs.py` | all | – | – | – | – | ✔ | — |
| `d01/v02/persistence.py` | `update_persistence` | – | – | – | – | ✔ | — |
| `d01/v02/perturbation.py` | 3 functions | – | – | – | – | ✔ | — |
| `d01/v02/reference.py` | `update_reference_and_scale` | – | – | – | – | ✔ | — |
| `d01/v02/reversal.py` | `compute_reversal_propensity` | – | – | – | – | ✔ | — |
| `d01/v02/snapshot.py` | all | – | – | – | – | ✔ | — |
| `d01/v02/state.py` | all | – | – | – | – | ✔ | — |
| `d01/v02/strength.py` | `compute_strength` | – | – | – | – | ✔ | — |
| `d01/v02/trace.py` | `TraceRecord` | – | – | – | – | ✔ | — |
| `d01/v02/uncertainty.py` | `compute_uncertainty` | – | – | – | – | ✔ | — |
| `d01/v02/volume.py` | `update_volume_influence` | – | – | – | – | ✔ | — |
| `d02/v02/builder.py` | `build_return_shape` | – | – | – | – | ✔ | — |
| `d02/v02/models.py` | `ReturnShape`, `ForwardSample` | – | – | – | – | ✔ | — |
| `d04/envelope/capturability_model.py` | `CapturabilityModelV0_2` | – | – | – | – | ✔ | — |
| `d04/models/capturability.py` | `CapturabilityResult` | – | – | – | – | – | **pydantic** |
| `d04/models/enums.py` | 6 enums | – | – | – | – | ✔ | — |
| `d04/models/envelope_context.py` | `EnvelopeContext` | – | – | – | – | – | **pydantic** |
| `pricing_pipeline/pipeline.py` | `PricingPipeline.process` | **✔** | – | – | – | – | — |
| `pricing_pipeline/derivatives.py` | `causal_quadratic` | **✔** | – | – | – | – | — |
| `pricing_pipeline/derivatives.py` | `derivative_state` | – | – | – | – | ✔ | — |
| `pricing_pipeline/dynamics.py` | `fit_f4`, `allocate_fit`, `valid_fit` | **✔** | – | – | – | – | — |
| `pricing_pipeline/projection.py` | `solve_cover` | **✔** | **✔** | – | – | – | — |
| `pricing_pipeline/numerical.py` | `build_numerical_row` | **✔** | **✔** | – | – | – | — |
| `price_engine/contracts.py` | all | – | – | – | – | ✔ | — |
| `price_engine/engine.py` | `PriceEngine` | – | – | – | – | ✔ | — |
| `price_engine/policy.py` | `EmissionPolicy` | – | – | – | – | ✔ | — |
| `price_engine/cockpit.py` | `PriceCockpitInterpreter` | – | – | – | – | ✔ | — |
| `unit_run/run_001.py` | harness | – | – | – | – | ✔ | — |
| `unit_run/run_pricing_001.py` | harness | – | – | – | – | ✔ | — |

Totals: **NumPy in 5 modules. SciPy in 2 modules. pydantic in 2 modules.
grpcio in 2 modules (one generated). sklearn: zero. pandas: zero.**
`sklearn` and `pandas` are not declared in `pyproject.toml` and appear nowhere in
the codebase.

## H.2 What each dependency actually does

**NumPy — 5 modules.** Per the instruction that importing NumPy does not by
itself justify remaining in Python, each usage was examined:

| Usage | Where | Actually needed for | Go substitutable? |
|---|---|---|---|
| `np.asarray`, `np.full`, `np.column_stack`, `np.arange`, `np.r_`, `np.linspace` | all 5 | array allocation and layout | Yes — `[]float64` and `mat.NewDense` |
| `np.isfinite`, `np.all`, `np.any`, `np.minimum`, `np.flatnonzero`, boolean masks | all 5 | element-wise predicates | Yes — explicit loops |
| `.mean(axis=0)`, `.std(axis=0)`, `.min/.max(axis=0)` | `dynamics.py` | column statistics over a 30×3 window | Yes — explicit loops; **`std` must be population (`ddof=0`)** |
| `np.linalg.lstsq` | `derivatives.py:86` | least squares on a 15×3 system, with rank check | Yes — `mat.SVD` |
| `np.linalg.solve` | `dynamics.py:100` | 4×4 linear solve | Yes — `mat.Dense.Solve` |
| `np.linalg.cond` | `dynamics.py:110` | 2-norm condition number of a 30×4 matrix | Yes — `mat.Cond(a, 2)` |
| `np.linalg.eigvals` | `numerical.py:88` | eigenvalues of a 3×3 companion matrix | Yes — `mat.Eigen` |
| `np.linalg.norm` | `projection.py:123`, `numerical.py:89` | 2-norm of small vectors | Yes — `floats.Norm` |
| `.reshape/.transpose/.ravel` | `projection.py` | flatten/unflatten the batched state | **Not needed** — production is single-index |

Verdict: **NumPy is a convenience layer over five small dense linear-algebra
operations, all of which gonum provides.** Nothing NumPy does here is
irreplaceable, and the array sizes (15×3, 30×4, 3×3, 4×4) are so small that
NumPy's vectorisation advantage is negative — per-call overhead dominates.

**SciPy — 2 modules, 2 functions:**
- `scipy.integrate.solve_ivp` (`projection.py:39`) — **the one genuine gap.**
- `scipy.linalg.expm` (`numerical.py:41`) — has a direct gonum equivalent
  (`mat.Dense.Exp`) using the same Higham algorithm family.

Verdict: **SciPy justifies exactly one Python retention, and it is `solve_ivp`.**

**pydantic — 2 modules.** Used only for declarative validation:
`ConfigDict(extra="forbid", allow_inf_nan=False)`, `Field(ge=..., le=...)`, and
one `model_validator` enforcing the provenance invariant. No mathematics. In Go
this becomes struct definitions plus an explicit `Validate() error` method.
Verdict: does not justify Python.

**grpcio — 2 modules.** Transport only. In a consolidated Go runtime the
SADE-side gRPC client disappears entirely, because ingestion moves in-process.
Verdict: does not justify Python; it is the thing being eliminated.

## H.3 The temporary Python boundary after first migration

Determined from evidence, aiming at the *smallest mathematically justified*
boundary rather than assuming whole pipelines must remain.

The Adaptive Pipeline needs **no** Python boundary — it is stdlib-only end to
end. The Pricing Pipeline needs a Python boundary only around `solve_cover`.

Smallest justified boundary:

```text
Go computes:  fit β (4), means (3), scales (3), minimum (3), maximum (3),
              initial state [p, p1, p2] (3), rtol, epsilon
                    │
                    ▼   ONE narrow call, ~22 float64 in
        ┌───────────────────────────────────┐
        │ Python RK45 routine               │
        │ scipy.integrate.solve_ivp         │
        │ method="RK45", t_span=(0,1)       │
        │ per-component atol vector         │
        │ t_eval = linspace(0,1,11)         │
        └───────────────────────────────────┘
                    │   ~35 float64 + status out
                    ▼   (11×3 trajectory, nfev, message)
Go computes:  instability check, D_local_maximum, envelope exit detection,
              eigenvalues, expm amplification, numerical row, PriceEngine,
              policy, cockpit, output IO_Vector
```

Interface size: approximately **22 float64 in, 33 float64 + 1 int + 1 string
out**. That is the entire remaining language boundary of the system.

Note deliberately: the instability check, `D_local_maximum`, and envelope-exit
detection currently live *inside* `solve_cover` (lines 120–137) but require no
SciPy. They should move to Go, shrinking the Python surface from 146 lines to
roughly 40.

## H.4 Wrapper microservices — assessment

Earlier architecture discussions considered `Adaptive_Pipeline_Wrapper` and
`Pricing_Pipeline_Wrapper`. Assessed against evidence.

### `Adaptive_Pipeline_Wrapper`

| Question | Finding |
|---|---|
| Why would it exist? | Only to keep the existing Python adaptive code running while Go is built. |
| Language | Python |
| Interface | Would need to accept a source row and return the full emission — a large payload including full DMO/FMO/ReturnShape dicts. |
| Lifecycle | Would need per-`IO_Vector` state affinity, because `AdaptiveEmitter` holds a 15-deque and D01 holds recursive state. A stateless request/response wrapper is **impossible** without externalising and shipping that state on every call. |
| Becomes unnecessary after Go migration? | **Yes, completely.** |
| Are direct narrow Python calls better? | Not applicable — there is no mathematical reason for any adaptive Python call to survive. |

**Classification: `NOT REQUIRED`.**

Evidence: the entire adaptive path (D01 966 + D02 184 + D04 239 + emitter 452 =
1,841 lines) is stdlib-only with no NumPy, no SciPy and no library that Go
lacks. Building a stateful Python wrapper service for code that has no reason to
be in Python would add a process boundary, a serialisation cost, and a
state-affinity routing problem, in exchange for nothing. It is strictly worse
than either (a) leaving the current Python monolith alone until Go is ready, or
(b) migrating the adaptive path first — which the classification suggests is the
*easiest* migration in the system.

### `Pricing_Pipeline_Wrapper`

| Question | Finding |
|---|---|
| Why would it exist? | To host `solve_ivp` during Phase 3–4. |
| Language | Python |
| Interface | If scoped to the whole pipeline: large, stateful (history arrays, `PolicyState`, `CockpitState`). If scoped to RK45 only: **stateless and tiny** — 22 floats in, ~35 out. |
| Lifecycle | Whole-pipeline scope → per-channel state affinity required. RK45-only scope → completely stateless, freely poolable, restartable, replaceable. |
| Becomes unnecessary after Go migration? | Yes, once Go Dormand–Prince passes equivalence. |
| Are direct narrow Python calls better? | **Yes, decisively.** |

**Classification: `TEMPORARY MIGRATION BRIDGE` — but only in its narrow,
RK45-only form.** A whole-pricing-pipeline wrapper is `NOT REQUIRED`.

The distinction matters enormously for the architecture. A stateless RK45
service can be scaled, load-balanced, restarted and eventually deleted without
touching runtime state. A stateful pricing-pipeline service becomes a permanent
partition-affinity constraint on the Go runtime and would be very hard to
remove.

**Are wrapper microservices needed initially? Only one, narrow, and only if the
Go RK45 is not written first.** If Phase 3 writes the Go Dormand–Prince
integrator up front and validates it against both SciPy and the analytic
`expm` solution, **zero wrapper services are ever needed.** That is worth
serious consideration, because it avoids introducing a process boundary that
must later be removed.

**Are wrapper microservices needed permanently? No.**

## H.5 SDX future role

Options were assessed against executable responsibilities, not the repository
name.

What SDX genuinely owns that is worth keeping:
- `router.go`'s partition/fan-out/fan-in/bounded-queue pattern — this is
  precisely the target concurrency model, already written and already tested.
- Lifecycle and graceful-shutdown handling in `main.go` — correct and reusable.
- Status reporting model (`PartitionState`, `SourceState` enums and the
  corresponding RPCs) — a good observability foundation.
- The discipline of forwarding source timestamps verbatim with no assigned
  semantics — a genuinely valuable design decision that must be preserved.

What is source-specific and must be generalised:
- `SDReader` is CSV-only with a hard-coded 6-column header (`reader.go:16`).
- The entity set is a compile-time slice (`main.go:23`).
- `MarketVector` is OHLCV-shaped (`sdx.proto:55–65`).
- `source_row_index` restarts at 0 on every `Read` call, so it is a
  per-request counter, not a durable stream sequence.
- One `SDReader` per entity holds a single mutable `status`, so two concurrent
  streams of the same entity corrupt each other's status
  (`reader.go:53` unconditionally sets `StateReading`).

**Recommendation: option C, with a staged path through option B.**

SDX should become **part of a consolidated `SADE_Go` runtime**, with its
`reader`/`router` packages generalised into a source-adapter layer and an
`IO_Vector` ingress/partitioning layer. Rationale from evidence:

1. The gRPC hop between SDX and SADE exists only because SADE is Python. Once
   the runtime is Go, that hop is pure cost — protobuf marshal + unmarshal on
   every vector, for an in-process data handoff.
2. `router.go` is not a "source-side" concern; it is the general partitioning
   and concurrency engine the whole runtime needs. Leaving it behind a network
   boundary means either duplicating it downstream or routing all channel
   ownership through a remote service.
3. Keeping SDX as a separate *deployed* service would satisfy none of the
   microservice-boundary criteria in §7: there is no independent scaling need
   (ingestion is I/O-bound and trivial), no fault-isolation benefit (a source
   failure already cancels everything), no language boundary once SADE is Go, no
   security boundary, and no separate deployment lifecycle.

**Should SDX remain separate from SADE_Go? No — not as a deployed network
service.** It should remain a separate *Go package boundary* (`ingress/source`,
`ingress/router`) within one runtime. A network boundary should be reintroduced
later only if and when a specific external source genuinely requires process
isolation — for example an unstable third-party SDK, a credential-isolation
requirement, or a source that must scale independently of processing.

## H.6 Multi-source ingestion

Current assumptions in code that block N-source ingestion:

| Assumption | Evidence | Required change |
|---|---|---|
| Sources are local CSV files | `reader.go:54` `os.Open(r.path)` | Introduce a `SourceAdapter` interface; CSV becomes one implementation |
| Fixed 6-column header | `reader.go:16, 65–67` | Per-adapter schema declaration |
| Payload is OHLCV | `sdx.proto:55–65`; `parseVector` | Generic payload (Part I) |
| Source set is compile-time | `main.go:23` | Runtime source registration/configuration |
| One reader per entity, single mutable status | `reader.go:36–40` | Per-stream reader instances, or status keyed by stream ID |
| Row index restarts per request | `reader.go:69` | Durable per-channel sequence assigned by ingress |
| `entity_id` is both identity and partition key | throughout | Separate `io_vector_id` (partition key) from source identity |
| Entity names are upper-cased and compared literally | `server.go:167`, `router.go:96` | Opaque channel identifiers; no case folding |

What N-source ingestion requires architecturally:

```text
SOURCE_000001 adapter ─┐
SOURCE_000002 adapter ─┤
        ...            ├──► ingress: normalise ──► IO_Vector(INPUT) ──► partition by
SOURCE_00000N adapter ─┘      + assign sequence        (canonical)        io_vector_id
                              + stamp ingest time
```

Each adapter owns only "produce raw records from my source". Normalisation to
`IO_Vector`, sequence assignment, ingress timestamping and partitioning are
shared, source-agnostic ingress responsibilities. No provider-specific
integration is proposed or implemented here.

---

# Part I — IO_Vector Investigation

## I.1 Field-level comparison

Three existing structures were compared against the proposed generic
`IO_Vector`.

| Concept | SDX `MarketVector` (`sdx.proto:55`) | SADE `NormalizedObservation` (`d01/v02/observations.py`) | SADE `source_row` dict (`pipeline.py:96`) | Available for `IO_Vector`? |
|---|---|---|---|---|
| Channel / partition identity | `entity_id` (string) | `entity_id` | `entity_id` | **Yes**, but financial-specific naming and semantics |
| Source identity | **absent** (implied by server config) | **absent** | **absent** | **No — gap** |
| Sequence | `source_row_index` (uint64, restarts per request) | `sequence_id` (int) | `source_row_number` (string) | **Partial** — not durable |
| Source timestamp | `source_timestamp` (string, verbatim) | `event_time` (float epoch) | `event_timestamp_utc` (string) | **Yes** |
| Ingress / receive timestamp | **absent** | `receive_time` — **fabricated as `= event_time`** (`normalizer.py:68`) | **absent** | **No — gap** |
| Payload | `open, high, low, close, volume` — financial | `price, volume, bid, ask, bid_size, ask_size` — financial | OHLCV strings | **Financial-specific** |
| Payload availability | **absent** | `availability_mask: dict[str,bool]` | **absent** | **Partial** |
| Data quality | **absent** | `source_quality` (float) | `data_valid` (string "true", **SADE-injected assumption**) | **Partial, fabricated** |
| Regime / session | **absent** | `session` (string) | `session_type` ("UNKNOWN", **SADE placeholder**) | **Partial, fabricated** |
| Provenance | **absent** | **absent** | **absent** | **No — gap** (exists only for D04 context, not vectors) |
| Direction / role (INPUT vs OUTPUT) | **absent** | **absent** | **absent** | **No — gap** |
| Processing lineage | **absent** | **absent** | **absent** | **No — gap** |
| Result lineage / parent | **absent** | **absent** | **absent** | **No — gap** |
| Status | partition-level only (`PartitionStatus`) | **absent** | **absent** | **No — per-vector gap** |
| Errors | partition-level only (`error_message`) | **absent** | **absent** | **No — per-vector gap** |
| Schema version | **absent** on the vector | D01 carries `dmo_schema_version`, `fmo_schema_version` | **absent** | **Partial** |

Identity fields that *do* exist, but only on downstream artifacts rather than on
vectors:

| ID | Where | What it identifies |
|---|---|---|
| `observation_id` | `emitter.py:225` | SHA-256 over `{physical_row, source_row_number, timestamp, ohlcv}` — a content hash of the input observation |
| `emission_id` | `emitter.py:312` | SHA-256 over the full emission core |
| `prior_context_ids` | `emitter.py:286` | the 15 `observation_id`s in the rolling context — **a genuine lineage edge** |
| `trace_id` | `model.py:295` | `"{entity_id}:{sequence}"` |
| `state_hash` | `model.py:57` | SHA-256 over the D01 state vector |
| `config_hash` | `config.py:161` | SHA-256 over the full D01 config |
| `rule_fingerprint`, `code_fingerprint` | `scientific_baseline.py:38–39` | frozen scientific provenance constants |
| `source_fingerprint` | `emitter.py:307` | equals `observation_id` |

This is a strong foundation. The system already has content-addressed
observation identity, emission identity, context lineage, state hashing and
config hashing. What it lacks is a **vector-level** identity that travels with
the data through the runtime, and any notion of output-to-input lineage.

## I.2 Can `MarketVector` evolve into `IO_Vector`?

Yes — by generalisation, not replacement. The structural shape is already right:
a partition key, a sequence, a source timestamp forwarded verbatim, and a
payload. Three changes are needed:

1. **Payload genericity.** `open/high/low/close/volume` as top-level typed
   fields is the only genuinely financial-specific part of the contract.
2. **Envelope completion.** Add source identity, ingress timestamp, role,
   lineage and per-vector status.
3. **Sequence durability.** `source_row_index` must become a per-channel
   monotonic sequence owned by ingress, not a per-request loop counter.

## I.3 Minimum generic `IO_Vector` concept

Deliberately minimal — only fields for which this investigation found an
executable requirement. Presented as a concept, not an implementation.

```text
IO_Vector
├── IDENTITY
│   ├── io_vector_id          unique identity of this vector
│   ├── channel_id            partition key; owner selection; ordering scope
│   │                         (generalises entity_id; opaque, no case folding)
│   └── sequence              monotonic per channel_id, assigned by ingress
│                             (generalises source_row_index, made durable)
│
├── ROLE
│   └── role                  INPUT | OUTPUT
│                             (required by §21; absent everywhere today)
│
├── PROVENANCE
│   ├── source_id             which source produced it (SOURCE_000001, ...)
│   │                         (absent today; implied by server configuration)
│   ├── source_timestamp      verbatim from source, uninterpreted
│   │                         (preserves the SDX discipline at sdx.proto:63)
│   └── ingest_timestamp      when the runtime first observed it
│                             (absent today; REQUIRED for any latency measurement)
│
├── PAYLOAD
│   ├── payload               generic typed values, source-schema-defined
│   │                         (generalises OHLCV; financial shape becomes one schema)
│   ├── payload_schema        schema identity + version
│   └── availability          which payload fields are actually present
│                             (generalises availability_mask)
│
├── QUALITY
│   └── source_quality        as supplied by the source, or explicitly UNKNOWN
│                             (today fabricated as data_valid="true")
│
├── LINEAGE
│   ├── parent_io_vector_id   immediate causal parent
│   ├── input_io_vector_id    originating INPUT vector for an OUTPUT vector
│   └── context_io_vector_ids contributing vectors (generalises prior_context_ids)
│
└── STATUS
    ├── status                per-vector processing status
    └── error                 per-vector error, if any
        (both exist today only at partition level, in PartitionStatus)
```

Explicitly **not** included, to avoid overdesign:
- No embedded scientific state. State belongs to the channel owner, not the
  vector.
- No embedded configuration or config hash. Runtime-level concern.
- No routing or topic metadata. Transport concern; must stay out of the vector
  for the portability reason in Part K.
- No instrument, ticker, symbol, asset-class or market concept anywhere.
- No decision or order fields. Out of scope.

## I.4 Input/Output symmetry

Investigated whether one canonical `IO_Vector` with an explicit role is
preferable to separate `Input_Vector` and `Output_Vector` types.

Evidence from current code:

- The system already produces heterogeneous outputs: `PriceEmission` (29
  fields), `PriceCockpitEmission` (16 fields), the adaptive emission dict, and
  `DMOOutput`/`FMOOutput`. These share almost no fields with the input
  `MarketVector`.
- Every current output carries a `symbol`/`entity_id` and a `timestamp` copied
  from its input (`policy.py:257–258`, `cockpit.py:213–214`), plus
  identity/lineage. So the *envelope* is shared even though the payload is not.
- `PriceEmission` and `PriceCockpitEmission` are already produced in the same
  causal step from the same channel and must be correlated — which is exactly
  what a shared lineage envelope provides.
- The Decision Engine is a declared future consumer. It will consume outputs and
  may produce further vectors. A single canonical envelope means adding it
  requires no new transport type.

**Advantages of one canonical `IO_Vector` with an explicit role:**
- One transport, one serialisation, one partitioning function, one ordering
  guarantee, one lineage mechanism, one Pub/Sub topic type, one Azure mapping.
- Output vectors are re-routable as inputs to downstream stages without
  conversion, which is what enables the target diagram's "Go processing
  resumes" step and the future Decision Engine boundary.
- Channel ownership and ordering logic is written once and applies to both
  directions.
- Matches how the code already behaves: outputs inherit channel identity and
  timestamp from inputs.

**Disadvantages:**
- The payload type must be a union or generic container, which loses static
  typing that separate types would give. In Go this means either a discriminated
  union with a schema tag, or `any` with type assertions — a real ergonomic cost.
- A vector can be constructed in an invalid combination (INPUT role with output
  lineage fields populated), which separate types would make unrepresentable.
  This must be handled by explicit validation.
- Some fields are meaningful in only one direction (`input_io_vector_id` on an
  INPUT vector is meaningless), so the struct carries dead fields in each
  direction.

**Recommendation: one canonical `IO_Vector` with an explicit role.**

```text
IO_Vector(INPUT)  ──►  processing  ──►  IO_Vector(OUTPUT)
```

The transport, ordering and lineage uniformity outweighs the typing cost,
particularly because the whole point of the target architecture is that one
runtime mechanism serves Adaptive, Pricing, future Volume, future semantic
interpretation and a future Decision Engine without redesign. The typing concern
should be mitigated by keeping the payload behind a schema-tagged accessor
rather than raw `any`, and by validating role/field consistency at construction.

## I.5 Lineage requirement

Every future output must be traceable to the input that caused it.

**What exists today:**
- `observation_id` — content hash of the input observation (`emitter.py:225`).
- `emission_id` — hash of the emission (`emitter.py:312`).
- `prior_context_ids` — the 15 contributing `observation_id`s
  (`emitter.py:286`). This is a real, working lineage edge.
- `source_fingerprint` — equals `observation_id`, carried on the emission
  (`emitter.py:307`), so **the adaptive emission is already traceable to its
  causing observation.**
- `trace_id` = `entity_id:sequence` (`model.py:295`).
- `state_hash`, `config_hash`, `rule_fingerprint`, `code_fingerprint`.
- `migration_hash_evidence.json` — module-level source→target SHA-256 lineage
  for the completed prior migration.

**What is missing:**
- No lineage at all on the *pricing* side. `PriceEmission` carries `symbol` and
  `timestamp` only (`contracts.py:131–132`); there is no `observation_id`,
  `emission_id` or parent reference. The link from a `PriceEmission` back to its
  adaptive observation exists **only** by `(symbol, timestamp)` coincidence —
  which is also the only thing `PriceEngine.observe` validates
  (`engine.py:103–110`). And because the pricing stage runs one observation
  behind (`active_index = index − 1`), correlating a pricing output with its
  adaptive row requires knowing that offset externally.
- No vector-level ID on `MarketVector` at all.
- No `parent_io_vector_id` or `input_io_vector_id` concept anywhere.
- No ingress timestamp, so lineage cannot carry timing.
- `physical_row = source_row_index + 2` is embedded in the `observation_id`
  hash (`emitter.py:227`), so current identity values depend on a legacy CSV
  row-numbering convention.

**Conceptual target, documented not implemented:**

```text
IO_Vector(INPUT)
   io_vector_id = V_in
        │
        ├── context_io_vector_ids = [V_in-15 ... V_in-1]   (generalises prior_context_ids)
        ▼
   adaptive processing  ──► IO_Vector(OUTPUT)
                              io_vector_id       = V_adapt
                              parent_io_vector_id = V_in
                              input_io_vector_id  = V_in
        │
        ▼
   pricing processing   ──► IO_Vector(OUTPUT)
                              io_vector_id       = V_price
                              parent_io_vector_id = V_adapt
                              input_io_vector_id  = V_in      <-- originating input preserved
```

Gap summary: the adaptive side has usable lineage; the pricing side has none;
neither has vector-level IDs or an ingress timestamp. Closing this gap requires
adding fields, not changing mathematics — with one caveat: **if
`observation_id`/`emission_id` values must remain byte-identical to the frozen
baseline, the `physical_row + 2` convention and the exact JSON canonicalisation
must be preserved.** That is a decision for human review, recorded in the risk
register.

---

# Part J — State, Partition, Concurrency, Pub/Sub, Backpressure

## J.1 Current state inventory

Every state-holding object found in either repository.

| # | State | Location | Keyed by | Scope | Bounded? | Runtime or Scientific |
|---:|---|---|---|---|---|---|
| 1 | `SDReader.status` | `reader.go:39` | one per entity, per reader instance | process | Yes | RUNTIME |
| 2 | `SDXRouter.statuses` | `router.go:49` | entity string | process | Yes | RUNTIME |
| 3 | `SDXRouter.configured` | `router.go:50` | — | process | Yes | RUNTIME |
| 4 | partition channels | `router.go:219` | entity, per request | per RPC | Yes (cap 10) | RUNTIME |
| 5 | `AdaptivePipeline._expected_index` | `pipeline.py:228` | implicit single entity | process | Yes | RUNTIME (ordering) |
| 6 | `AdaptivePipeline._vectors_received` | `pipeline.py:229` | implicit single entity | process | Yes | RUNTIME |
| 7 | `AdaptivePipeline._rows` | `pipeline.py:227` | implicit single entity | process | **NO** | RUNTIME |
| 8 | `AdaptiveEmitter.context` | `emitter.py:140` | implicit single entity | process | Yes (`deque(maxlen=15)`) | **SCIENTIFIC** |
| 9 | `AdaptiveEmitter.position_state` | `emitter.py:141` | implicit single entity | process | Yes | **SCIENTIFIC** |
| 10 | `AdaptiveEmitter.previous_decision` | `emitter.py:142` | implicit single entity | process | Yes | **SCIENTIFIC** |
| 11 | `AdaptiveEmitter.completed_count` | `emitter.py:143` | implicit single entity | process | Yes | **SCIENTIFIC** (gates INITIALIZING→ACTIONABLE) |
| 12 | `AdaptiveEmitter.emissions` | `emitter.py:144` | — | process | **NO** | RUNTIME (diagnostic) |
| 13 | `AdaptiveEmitter.initialization` | `emitter.py:145` | — | process | **NO** | RUNTIME (diagnostic) |
| 14 | `AdaptiveEmitter.adaptation_audit` | `emitter.py:146` | — | process | **NO** | RUNTIME (diagnostic) |
| 15 | `AdaptiveEmitter.feedback_audit` | `emitter.py:147` | — | process | **NO** | RUNTIME (diagnostic) |
| 16 | `AdaptiveEmitter._last_source_time` | `emitter.py:148` | — | process | Yes | **SCIENTIFIC** (causality guard) |
| 17 | `D01V02Model.state` (`RuntimeState`) | `model.py:36` | `entity_id` field | process | Yes | **SCIENTIFIC** — the core recursive state |
| 18 | `RuntimeState.last_observation` | `state.py:37` | — | process | Yes | **SCIENTIFIC** |
| 19 | `RuntimeState.parameter_state` | `state.py:38` | — | process | Yes | **SCIENTIFIC** (adapted `ref_alpha`) |
| 20 | `D01V02Model.trace_records` | `model.py:41` | — | process | **NO** | RUNTIME (diagnostic) |
| 21 | `D01V02Model.config` / `config_hash` | `model.py:34–35` | — | process | Yes | CONFIGURATION (immutable) |
| 22 | `PricingPipeline._source_row_index` | `pipeline.py:192` | implicit single entity | process | **NO** | RUNTIME (ordering) |
| 23 | `PricingPipeline._timestamps` | `pipeline.py:193` | — | process | **NO** | **SCIENTIFIC** (history) |
| 24 | `PricingPipeline._times_minutes` | `pipeline.py:194` | — | process | **NO** | **SCIENTIFIC** (history) |
| 25 | `PricingPipeline._opens/_highs/_lows/_closes/_volumes` | `pipeline.py:195–199` | — | process | **NO** | **SCIENTIFIC** (history) |
| 26 | `PricingPipeline._policy_state` | `pipeline.py:175` | — | process | Yes (2 fields) | **SCIENTIFIC** |
| 27 | `PricingPipeline._cockpit_state` | `pipeline.py:178` | — | process | Yes (6 fields) | **SCIENTIFIC** |
| 28 | `PricingPipeline._summary` (5 `Counter`s) | `pipeline.py:201` | — | process | Bounded by label cardinality | RUNTIME |
| 29 | `EmissionPolicy.config` | `policy.py:142` | — | process | Yes | CONFIGURATION (frozen calibration) |
| 30 | `PriceCockpitInterpreter.config` | `cockpit.py:108` | — | process | Yes | CONFIGURATION |

Global state, mutable singletons, thread/process assumptions:

- **No module-level mutable globals** in SADE. All state is instance-attached.
  This is a significant positive finding — it means per-channel instances are
  already viable without untangling globals.
- However, **every SADE state object is keyed implicitly by "the one entity this
  process is handling"**. `AdaptivePipelineConfig.entity` is a single string;
  `PricingPipelineConfig.entity` is a single string. The current design is
  strictly one channel per process.
- `RuntimeState.entity_id` exists as a field but is never used as a lookup
  key — it is carried for labelling only (`model.py:295`, `outputs.py`).
- Thread safety: SADE has **no locks and no thread-safety provisions anywhere**,
  because it is single-threaded by construction. This is fine for the Go target
  (each channel owner is a single goroutine), but means the Python code cannot be
  made concurrent by simply adding threads.
- SDX's `SDReader` and `SDXRouter` use `RWMutex` correctly for their own fields,
  but `SDReader.status` is a **single mutable status per source**, so two
  concurrent streams over the same source interleave status writes
  (`reader.py:53` sets `StateReading` unconditionally on entry).

## J.2 Runtime state vs scientific state

Applying the required separation:

**SCIENTIFIC STATE** — must move with the mathematics; authoritative; determines
numerical output:

| State | Size |
|---|---|
| `D01 RuntimeState` (incl. `StateVector`, `HalfLifeState`, `parameter_state`, `last_observation`) | ~20 scalars + 1 observation |
| `AdaptiveEmitter.context` (rolling 15) | 15 records × ~20 fields |
| `AdaptiveEmitter.position_state`, `previous_decision`, `completed_count`, `_last_source_time` | 4 scalars |
| `PolicyState` | 2 fields |
| `CockpitState` | 6 fields |
| Pricing price/time history | needs only the last `f4_window + derivative_window` ≈ 45 values per series |

Total genuinely required scientific state per channel: **on the order of 10–20 KB
if bounded.** Today it is unbounded.

**RUNTIME STATE** — belongs in Go regardless of where mathematics lives:

Ordering counters (`_expected_index`, `_source_row_index`), vectors-received
counters, all summary `Counter`s, all audit and trace collections, all emission
archives, partition/source status, channel lifecycle, queue depths.

**Configuration** — immutable, replicated freely, never mutated at runtime:
`D01V02Config` and its 15 sub-configs, `PolicyConfig`, `CockpitPolicyConfig`,
`PricingPipelineConfig`, baseline fingerprints.

**Principle for the target:** Go owns all runtime state. Scientific state is
owned by whichever language holds the mathematics — and after migration that is
Go for everything, since RK45 (the only Python residue) is **stateless**. This
is a very favourable outcome: **no authoritative state ever needs to live in
Python, and no state needs to be duplicated across the language boundary.**

State objects requiring an explicit ownership decision during migration:

1. `D01 RuntimeState` — moves to Go with D01. Must be per-channel.
2. `AdaptiveEmitter.context` — moves to Go. Bounded already.
3. `PolicyState` / `CockpitState` — move to Go. Already pure transitions.
4. Pricing history arrays — move to Go **and must be bounded**, replacing
   append-forever lists with ring buffers.
5. Audit/trace/emission collections — become Go telemetry or persisted output
   streams, **not** in-memory accumulation.
6. `SDReader.status` — must become per-stream, not per-source.

## J.3 What must change for thousands of independently stateful channels

| Blocker | Evidence | Required change |
|---|---|---|
| One channel per process | `AdaptivePipelineConfig.entity: str` | Per-channel state instance keyed by `channel_id`, owned by one goroutine |
| Unbounded history | `pipeline.py:192–199` append-only | Ring buffers sized to `max(derivative_window, f4_window) + margin` |
| Unbounded audits/traces/emissions | items 12–15, 20 in J.1 | Emit as telemetry/output; do not retain |
| O(n²) recomputation | `derivatives.py:81`, `dynamics.py:89` | Compute only the current index — mathematically identical |
| Cross-channel failure coupling | `router.go:242` shared `cancel()` | Per-channel cancellation scope |
| Cross-channel head-of-line blocking | `router.go:183` unbuffered shared fan-in | Per-channel pipelines; bounded fan-in only where a genuine merge is needed |
| Per-source mutable status | `reader.go:39` | Status keyed by stream/channel |
| Per-request sequence restart | `reader.go:69` | Durable per-channel sequence |
| Hot-path hashing/deepcopy | `emitter.py:262, 312–314, 317, 362–368` | Compute identity once; avoid deep copies |
| Compile-time channel set | `main.go:23` | Runtime channel registration |

Ordering requirement: causal order must be preserved **within** each channel
while different channels proceed concurrently. The current code already has both
halves in isolation — SADE enforces strict per-channel monotonic sequence
(`pipeline.py:128–132`, `observations.py:45–51`, `emitter.py:195`), and SDX
already preserves per-partition order through independent producer/forwarder
goroutines, asserted by `TestStreamVectorsReturnsFirst100RowsForFiveEntities`
(`server_test.go:101`), which verifies that with five concurrent partitions each
entity's `source_row_index` arrives in strict sequence.

**This is the single most encouraging piece of evidence in the investigation:
concurrent multi-channel ingestion with per-channel ordering already works and is
already tested. What does not exist is concurrent multi-channel *processing*.**

## J.4 Candidate Go concurrency model

Evidence-based, built from what SDX already does, avoiding unnecessary
machinery.

```text
                          SADE_Go single process

  SOURCE_000001 adapter ──┐
  SOURCE_000002 adapter ──┤   each adapter = 1 goroutine per active source
        ...               │
  SOURCE_00000N adapter ──┘
            │
            │  normalise -> IO_Vector(INPUT); assign channel_id, sequence,
            │               ingest_timestamp, io_vector_id
            ▼
   ingress channel   chan IO_Vector   (bounded, shared)
            │
            ▼
   ┌────────────────────────────────────────────────┐
   │ partition / ownership logic                    │
   │   owner = registry[channel_id]                 │
   │   create owner on first sight; retire on idle  │
   │   FAN-OUT by channel_id -- never round-robin   │
   └────────────────────────────────────────────────┘
            │            │            │
            ▼            ▼            ▼
   chan IO_Vector  chan IO_Vector  chan IO_Vector      one bounded channel
   (channel A)     (channel B)     (channel N)         per IO_Vector channel
            │            │            │
            ▼            ▼            ▼
   ┌──────────────┐┌──────────────┐┌──────────────┐
   │ owner        ││ owner        ││ owner        │   ONE goroutine per channel.
   │ goroutine A  ││ goroutine B  ││ goroutine N  │   Owns ALL state for that
   │              ││              ││              │   channel exclusively.
   │ D01 state    ││ D01 state    ││ D01 state    │   NO locks needed: state is
   │ ctx deque 15 ││ ctx deque 15 ││ ctx deque 15 │   reachable from exactly one
   │ price ring   ││ price ring   ││ price ring   │   goroutine, so ordering is
   │ PolicyState  ││ PolicyState  ││ PolicyState  │   guaranteed by construction
   │ CockpitState ││ CockpitState ││ CockpitState │   and races are impossible.
   └──────┬───────┘└──────┬───────┘└──────┬───────┘
          │               │               │
          │  (Phase 3-4 only, if Go RK45 not yet ready)
          │  bounded semaphore -> narrow stateless Python RK45 call
          │
          ▼               ▼               ▼
   chan IO_Vector(OUTPUT) per channel
          │               │               │
          └───────────────┴───────────────┘
                          │  FAN-IN (bounded, only where a merge is required:
                          │  persistence, telemetry, downstream consumers)
                          ▼
                  output sink(s) / Pub/Sub topics
```

Design decisions and why the evidence supports them:

**One goroutine per `IO_Vector` channel, owning all state for that channel.**
This is the decisive choice. It gives per-channel causal ordering *for free* —
ordering is a consequence of single-goroutine execution, not of added
synchronisation. It requires no mutexes on scientific state, because that state
is reachable from exactly one goroutine. It maps directly onto the existing
code, since every SADE state object is already instance-attached with no
globals (J.1). And it makes the Python-boundary problem tractable: the owner
goroutine blocks on its own RK45 call without blocking any other channel.

**Fan-out strictly by `channel_id`, never round-robin.** Round-robin
distribution would destroy per-channel ordering. This is why a generic worker
pool is the wrong pattern here and why §6's instruction to avoid worker pools is
technically correct rather than merely stylistic: a worker pool over a shared
queue cannot preserve per-key ordering without reintroducing per-key
serialisation, at which point it *is* the owner-goroutine model with extra
indirection.

**No worker-pool manager.** The one place a bounded pool is justified is
limiting concurrent calls into a *stateless* external routine — i.e. the
temporary Python RK45 bridge, where a `golang.org/x/sync/semaphore` (already in
the module graph) suffices. That is admission control, not a worker pool over
stateful work.

**No shared mutable global state.** Achievable because the Python code has none
to inherit.

**No lock-heavy orchestration.** The only shared structures are the channel
registry (read-mostly; `sync.Map` or `RWMutex`) and telemetry counters (atomics).
Neither is on the numerical path.

**No per-vector OS process, no per-channel network service.** Nothing in the
evidence requires either.

**Goroutine budget:** N owners + N source adapters (typically far fewer than N)
+ a small fixed set of ingress/egress/telemetry goroutines. At N = 1,000, roughly
1,000–2,100 goroutines — the same order as SDX's existing 2N+2 pattern, and a
trivial load for the Go scheduler.

## J.5 Pub/Sub model

The illustrative progression in the task was assessed against required stages.

Stages that actually exist in executable code today:

| Candidate event | Real stage in code? | Evidence |
|---|---|---|
| `IO_Vector_Received` | Yes | ingress + validation (`pipeline.py:242–243`) |
| `Adaptive_State_Updated` | Yes | `D01.step` + emitter state commit + D02 + D04 produce a complete adaptive result |
| `Pricing_State_Updated` | Yes | `PricingPipeline.process` produces `PriceEmission` + next `PolicyState` |
| `Output_Vector_Ready` | Yes | `PriceCockpitEmission` and/or `PriceEmission` serialised to output |

So the four illustrative stages do map onto genuine boundaries. But the
investigation finds the intermediate two should **not** be Pub/Sub events in the
local runtime, for a concrete reason.

Analysis per stage:

| Stage | Required? | Local mechanism | Fan-out? | Fan-in? | Ack? | Causal sequence enforcement? | Persistent state? |
|---|---|---|---|---|---|---|---|
| Ingress → partition | **Yes** | bounded `chan IO_Vector` | **Yes** — by `channel_id` | No | No (in-process) | Yes — sequence assigned here | No |
| Adaptive → Pricing | **Yes, logically** | **direct function call inside the owner goroutine** | No | No | No | Guaranteed by single-goroutine execution | Owner-local |
| Pricing → Output | **Yes** | `chan IO_Vector(OUTPUT)` | Yes — persistence, telemetry, future consumers | Yes — bounded merge | No | Per channel, by construction | No |
| Output → consumers | **Yes** | Pub/Sub topics | **Yes** | No | **Yes**, for external/durable consumers | Per channel | Consumer-side |

**Adaptive → Pricing should be a direct in-goroutine call, not a channel or
topic.** The evidence: `run_pricing_001.py:118–119` already calls
`adaptive.process_vector(vector)` then `pricing.process(adaptive_row)`
synchronously in one loop; the pricing stage requires the adaptive row for the
*same* channel in strict order, and pricing state (`PolicyState`,
`CockpitState`) is per-channel and sequential. Putting a channel between them
would add scheduling latency and a queue to manage while providing no
concurrency benefit — the two stages can never run in parallel for the same
channel anyway, because pricing depends on adaptive output for the same
observation. This is precisely the "logical Go component, not a deployed
service, and not even a separate channel" case that §7 asks to identify.

Where Pub/Sub genuinely adds value locally:
1. **Ingress fan-out by channel** — the essential distribution point.
2. **Output distribution** — multiple independent consumers (persistence,
   telemetry, cockpit, future Decision Engine, future Volume path) need the same
   output vector. This is real fan-out and the reason the target diagram's
   Pub/Sub layer is justified.
3. **Future path addition** — a Volume or semantic path subscribing to the same
   `IO_Vector(INPUT)` stream without modifying the adaptive path. This is what
   makes the architecture extensible as required.

Where fan-out is required: ingress → channel owners (by key); output → multiple
consumer families.

Where fan-in is required: channel outputs → persistence; channel outputs →
telemetry aggregation. Both must be **bounded**, and — critically — must not
recreate the current `router.go:183` unbuffered single-channel merge that
couples all channels together.

Where acknowledgement is needed: only at durable/external boundaries. In-process
channel sends are the acknowledgement.

Where causal sequence must be enforced: at ingress (sequence assignment) and
within each owner (guaranteed by construction). Cross-channel ordering is not
required and should not be promised.

## J.6 Backpressure

### Current behaviour

**SDX streaming:** Backpressure exists and works, by blocking sends.
`SDReader.Read` does `select { case output <- vector: ; case <-ctx.Done(): }`
(`reader.go:85–90`) into a channel of capacity `DefaultCapacity = 10`
(`router.go:15, 219`). When the queue fills, the producer goroutine parks and
stops reading the file. Memory stays bounded. The router surfaces this as
`PARTITION_STATE_BACKPRESSURED` when `depth == capacity` (`router.go:143, 149`).
`TestAAPLBackpressureDoesNotStopMSFT` (`router_test.go:52`) proves one
partition can sit full while another drains to completion.

**gRPC buffering:** Not configured. `grpc.NewServer()` is called with no options
(`main.go:43`), so default HTTP/2 flow-control windows apply. No
`MaxConcurrentStreams`, no window tuning, no send/recv message size limits, no
keepalive settings. Flow control therefore exists (HTTP/2 provides it) but is
entirely default and unmeasured.

**Python processing latency:** NOT YET MEASURED. Structurally, the Python
consumer is a single-threaded blocking loop
(`pipeline.py:300` `for vector in stream:`), and the current per-observation
cost *grows with stream length* because of the O(n²) recomputation. So the
consumer gets progressively slower as a run proceeds.

**Channel buffer implications:** `DefaultCapacity = 10` is a compile-time
constant with no configuration path. The fan-in channel is **unbuffered**
(`router.go:183`).

**Slow scientific routines:** `solve_ivp` is invoked for ~55% of observations
and is the most expensive single step.

**What happens when one channel is slow:** At the `RoutePartitions` layer,
nothing — other partitions continue. At the `Route`/`StreamVectors` layer, which
is what the service actually exposes: **all channels stall.** Every forwarder
sends into the same unbuffered channel drained by a single `stream.Send` loop, so
one slow consumer blocks every forwarder. And because `stream.Send` for *all*
entities is serialised through one loop, a single slow channel is a global
head-of-line block.

**Can one slow channel block unrelated channels?** **Yes, today.** Both through
the unbuffered fan-in and through shared cancellation (`router.go:242`).

### Proposed partition-aware backpressure strategy

Documented, not implemented.

1. **Bounded queue per channel, not per pipeline.** Each owner goroutine has its
   own bounded input channel. Backpressure is therefore inherently
   partition-scoped: a slow channel fills only its own queue.
2. **Blocking send as the default mechanism.** Keep SDX's existing approach —
   it is correct, simple, and already tested. A blocked send parks a goroutine at
   near-zero cost.
3. **Per-source admission control.** When a channel's queue is full, the source
   adapter for that channel pauses. Because adapters are per-source and channels
   are per-key, one source feeding many channels needs a policy decision:
   pause the whole source (simple, couples channels) or buffer per channel
   (bounded, more memory). This is a genuine design decision requiring human
   input, and it is the one place where partition-aware backpressure is
   non-obvious.
4. **Explicit overflow policy per channel, chosen deliberately.** Options:
   block (preserves every observation, propagates pressure upstream); drop
   oldest (bounded latency, loses causal continuity — **unsafe**, because D01 is
   a recursive state model and dropping an observation silently changes the
   scientific trajectory); drop newest (same objection). Recommendation:
   **block, and never drop**, because the mathematics is causally recursive.
   `assert_causal_sequence` (`observations.py:45`) and the strict index checks
   would reject a gap anyway — so dropping does not merely degrade accuracy, it
   *fails*. This is an important scientific constraint on the backpressure
   design.
5. **No cross-channel cancellation.** Replace the shared `cancel()` with a
   per-channel cancellation scope so a failing channel is isolated. This is a
   direct fix to `router.go:242`.
6. **Bounded fan-in at the output, with per-channel egress queues.** Never a
   single unbuffered merge channel.
7. **Observable pressure.** Extend the existing `PartitionState` model
   (which already has a `BACKPRESSURED` state) to per-channel queue depth,
   time-in-backpressure, and high-water marks, exported via OpenTelemetry
   metrics (already in the module graph).
8. **Configurable capacity.** `DefaultCapacity` must become configuration, sized
   from measured processing latency once measurement exists.

---

# Part K — Scale, Latency, Azure, Hard-Coding

## K.1 Scale target analysis

Target: thousands of concurrently active `IO_Vector` channels. Explicitly **not**
thousands of OS processes, microservices, or Python workers.

**Goroutine count.** The owner-per-channel model needs N owner goroutines plus a
small fixed overhead. At N = 1,000: ~1,000 owners + source adapters + ingress +
egress + telemetry ≈ 1,050–2,100 goroutines. SDX already runs 2N+2 for N
partitions, so the pattern is established. Go goroutines start with ~2 KB stacks
growing on demand; at 4–8 KB average, 2,000 goroutines is ~8–16 MB. **Not a
constraint.**

**Channel ownership.** One bounded input channel and one bounded output channel
per owner. At capacity 10 and a pointer-sized element, per-channel queue
overhead is trivial. The registry mapping `channel_id → owner` is read-mostly.

**Partition mapping.** Direct: `channel_id` → owner goroutine. No hashing ring,
no rebalancing, no consistent-hashing complexity needed locally, because all
owners are in one process and the registry is authoritative. Hashing becomes
relevant only when distributing across processes or Azure partitions (K.4).

**State memory.** Bounded per-channel scientific state, from J.2:
D01 `RuntimeState` ~20 scalars plus one observation; rolling context 15 records ×
~20 fields; pricing ring buffers ~45 values × 5–8 series; `PolicyState` 2
fields; `CockpitState` 6 fields. Estimated **10–20 KB per channel**, so
**10–20 MB for 1,000 channels** and 100–200 MB for 10,000. Comfortable.

The same figure with *current* unbounded behaviour is unbounded — this is the
difference between feasible and infeasible, and it is a code problem, not an
architecture problem.

**Ordering.** Guaranteed per channel by single-goroutine ownership; not promised
across channels. Matches the requirement exactly.

**Scheduling.** Go's work-stealing scheduler multiplexes goroutines onto
`GOMAXPROCS` OS threads. Channels are mostly idle (blocked on receive), so
runnable goroutines at any instant is bounded by arrival rate, not channel count.
Note: `GOMAXPROCS` is never set anywhere in SDX; the default (number of CPUs) is
used.

**Expected bottlenecks, in order of severity:**

1. **The O(n²) recomputation.** Dominates everything else. At observation *n*,
   `causal_quadratic` performs ~(n−14) least-squares fits and `fit_f4` performs
   ~(n−30) 4×4 solves plus 30×4 condition numbers — to produce **one** usable
   result. After 1,000 observations, each new observation triggers ~1,000 fits.
   Multiply by 1,000 channels and the runtime does ~10⁶ redundant fits per
   observation cycle. **This must be fixed first, and fixing it is
   mathematically free.**
2. **The Python boundary, if RK45 is retained cross-process.** ~55% of
   observations require a `solve_ivp` call. At 1,000 channels × 1 obs/s that is
   ~550 calls/s crossing a process boundary, each with serialisation, scheduling
   and GIL contention if a shared interpreter is used. Since CPython holds a
   global lock, concurrent RK45 calls into one interpreter serialise; scaling
   requires multiple interpreter processes, which is exactly the "thousands of
   Python workers" outcome the objective forbids. **This is the strongest
   argument for writing the Go RK45 sooner rather than later.**
3. **Unbounded memory growth.** Currently a hard failure mode, not a slowdown.
4. **Serialisation overhead.** Four `json.dumps` + SHA-256 passes per
   observation over large nested structures (`emitter.py`). In Go this is
   avoidable entirely for internal state, and needed only for output identity.
5. **gRPC overhead for the SDX→SADE hop.** Every vector is marshalled and
   unmarshalled. Consolidating into one Go process removes it completely. This is
   free performance with no scientific implication.
6. **Cross-channel head-of-line blocking.** The unbuffered fan-in at
   `router.go:183` serialises all channels through one send loop.
7. **pydantic model construction** per observation — two validated models per
   observation, eliminated by moving to Go structs.

Notably, the *actual numerical work* — a 15×3 least squares, a 4×4 solve, a
30×4 condition number, a 3×3 eigen-decomposition, a 3×3 matrix exponential, one
3-dimensional non-stiff ODE over a unit interval — is tiny. **The bottlenecks are
all structural, not mathematical.** That is a favourable finding: they are
fixable without touching the science.

## K.2 Near-real-time definition and latency budget framework

No numerical SLA is invented. Instead, the latency components are identified and
the measurement gap is stated.

**Measurement status: NOT YET MEASURED, everywhere.** A scan for `Benchmark`,
`time.Since`, `latency`, `throughput`, `elapsed` and `perf_counter` across both
repositories found timing instrumentation in exactly one file:
`sade/adaptive_emitter/emitter.py`, which records `direct_lifecycle_ns` and a
`component_lifecycle_ns` map with keys `SOURCE_ADMISSION`, `D01`, `D02`,
`FOUR_FACTOR`, `ADAPTIVE_DECISION` (lines 186–267, 303–306). These values are
embedded in each emission but **never aggregated, never summarised, and never
persisted** — `_build_record` (`pipeline.py:149–183`) does not include them, and
neither summary JSON contains any timing field. There are **zero Go benchmarks**
in SDX.

Additionally, **there is no ingress timestamp anywhere in the system**, so
end-to-end latency is currently *unmeasurable in principle*, not merely
unmeasured. `NormalizedObservation.receive_time` is fabricated as
`= event_time` (`normalizer.py:68`), and `MarketVector` has no receive field. Any
latency programme must first add an ingress timestamp — which is why
`ingest_timestamp` is in the minimum `IO_Vector` (Part I.3).

**Latency budget framework:**

| # | Component | Current owner | Measurable today? | Notes |
|---:|---|---|---|---|
| 1 | Source arrival → source read | SDX `SDReader.Read` | No | file I/O; for live sources this becomes network + provider latency |
| 2 | Vector construction / normalisation | `parseVector` | No | string→float parsing |
| 3 | Ingress admission + sequence assignment | does not exist yet | No | **requires `ingest_timestamp`** |
| 4 | Partition routing / owner dispatch | `router.go` fan-out | No | channel send |
| 5 | Queue / channel wait | partition channel (cap 10) | Partially — depth is exposed via `GetRouterStatus` | depth is observable; *time* in queue is not |
| 6 | Transport + serialisation (SDX→SADE) | gRPC | No | **eliminated by consolidation** |
| 7 | Adaptive computation | D01+D02+D04 | **Yes, partially** — `component_lifecycle_ns` exists but is discarded | the one instrumented stage |
| 8 | Adaptive identity/serialisation overhead | `canonical_sha256` ×4 | No | included in `direct_lifecycle_ns` |
| 9 | Pricing derivative + F4 computation | `causal_quadratic`, `fit_f4` | No | **currently grows with stream length** |
| 10 | Python boundary crossing (future) | RK45 bridge | No | does not exist yet |
| 11 | RK45 integration | `solve_cover` | No | ~55% of observations |
| 12 | Numerical assembly (eigen + expm) | `build_numerical_row` | No | |
| 13 | Policy + cockpit classification | `EmissionPolicy`, cockpit | No | pure comparisons; expected negligible |
| 14 | Output construction | `_build_record`, `as_dict` | No | |
| 15 | Output egress / persistence | CSV/JSON write | No | currently batch-at-end, not streaming |
| 16 | **Structural pipeline offset** | `active_index = index − 1` | N/A — deterministic | **one full observation interval of inherent latency**, independent of compute speed |

Component 16 deserves emphasis: the pricing stage is *architecturally* one
observation behind. If observations arrive once per minute, pricing output for an
observation is available only after the next observation arrives — a ~60 s
inherent latency that no amount of Go optimisation removes. Any "near real time"
definition must state whether this offset is acceptable or whether the
mathematics must change (which would be a scientific change, out of scope here).

**What should be measured before claiming near real time:**

Per component above, at minimum: p50/p95/p99 latency; per-channel throughput
(vectors/s); aggregate throughput at target channel count; queue depth and
time-in-queue distributions; goroutine count and scheduler latency; heap size
and allocation rate per channel; GC pause distribution; RK45 calls/s and
per-call latency; Python boundary concurrency and queueing if the bridge exists;
and end-to-end `ingest_timestamp → output emit` distribution. None of this is
possible until an ingress timestamp and stage instrumentation exist.

## K.3 Azure target mapping

No Azure implementation. Conceptual mapping only.

The governing principle from the evidence: **scientific semantics must not
depend on transport.** Today they accidentally do not — the mathematics consumes
a `NormalizedObservation`, which is constructed from a dict, which is
constructed from a protobuf message. The mathematics never touches gRPC. That
separation is worth preserving deliberately.

```text
LOCAL                                    AZURE
─────                                    ─────
source adapters (goroutines)      ──►    source adapters in Azure compute
        │                                        │
        ▼                                        ▼
ingress chan IO_Vector            ──►    Event Hubs (ingress ONLY)
        │                                   partition key = channel_id
        ▼                                        ▼
partition/ownership registry      ──►    Event Hubs partition -> consumer
        │                                   (one consumer per partition)
        ▼                                        ▼
per-channel typed Go channels     ──►    UNCHANGED: local Go channels
        │                                   inside each consumer instance
        ▼                                        ▼
owner goroutine per channel        ──►   UNCHANGED: owner goroutine per channel
   (D01/D02/D04/pricing/state)             (identical code, identical state)
        │                                        │
        ▼                                        ▼
output chan IO_Vector             ──►    Event Hubs / Service Bus (egress)
        │                                   for durable downstream consumers
        ▼                                        ▼
output sinks                      ──►    Azure storage / downstream services
```

The essential claim: **only the ingress and egress edges change.** The processing
graph, the ownership model, the ordering guarantee and the scientific code are
identical locally and on Azure. Each Azure consumer instance runs the same Go
runtime, owning the subset of channels mapped to its assigned partitions.

## K.4 Azure Event Hubs — where it fits and where it does not

Assessed conceptually against actual runtime needs, and explicitly resisting the
temptation to insert it everywhere because Azure is the target.

**Where Event Hubs adds real value:**

| Use | Why the evidence supports it |
|---|---|
| **Source ingress** | Decouples source availability from processing availability. Today a source failure cancels everything (`router.go:242`); a durable log removes that coupling entirely. |
| **Partition distribution across instances** | Event Hubs partition keys map naturally onto `channel_id`. This is the mechanism that lets channel ownership span multiple compute instances while preserving per-key ordering — Event Hubs guarantees ordering *within* a partition, which is exactly the guarantee the mathematics requires. |
| **Cross-process / cross-host transport** | Once the runtime scales beyond one instance, this is the transport. |
| **Replay** | Genuinely valuable and currently impossible. D01 is a recursive state model, so reproducing a scientific trajectory requires replaying the exact input sequence from the beginning. A durable ordered log makes deterministic replay possible — which is *also* the foundation for the equivalence testing in Part L. This may be the single most underrated benefit. |
| **Buffering / burst absorption** | Absorbs source bursts without unbounded in-process memory. |
| **Scale-out** | Adding partitions and consumer instances is the horizontal scaling path. |

**Where Event Hubs must NOT sit:**

| Anti-placement | Why |
|---|---|
| **Between Adaptive and Pricing** | These are sequential stages on the *same* channel that can never run in parallel (pricing needs adaptive output for the same observation, and both mutate per-channel state). Inserting a network log here would add milliseconds of latency and a partition-affinity problem in exchange for zero concurrency. It would also split per-channel scientific state across two consumers — the worst possible outcome. Keep this a direct in-goroutine call. |
| **Between D01, D02 and D04** | These are three function calls inside one scientific step (`emitter.py:207–216`), sharing state and requiring bitwise-coupled values (recall the D02/D04 exact-equality check in Part D.3). A transport here would be actively harmful. |
| **Between owner goroutine and its own state** | State is goroutine-local by design. |
| **Around the RK45 call** | A stateless 22-floats-in/33-floats-out numerical routine invoked ~550×/s does not belong behind a durable messaging log. If a boundary is needed, it is a local IPC or in-process call — and preferably neither, once the Go integrator exists. |
| **Between every internal processing stage generally** | This is the premature-decomposition trap §7 warns about. Local Go channels cost nanoseconds; Event Hubs costs milliseconds plus operational complexity. Internal stage boundaries should remain channels or function calls. |

**Where local Go channels remain preferable:** every intra-instance hop —
ingress fan-out to owners, adaptive→pricing, owner→egress, telemetry
collection. Local channels are nanosecond-scale, need no serialisation, need no
credentials, and cannot fail independently.

## K.5 Local → Azure portability requirements

Abstractions that must remain transport-independent so local channels can become
Azure transport without rewriting scientific code:

| Abstraction | Requirement | Current status |
|---|---|---|
| **Vector representation** | `IO_Vector` must be a domain type, not a protobuf-generated type. Serialisation must be a boundary concern. | **At risk.** SDX passes `*sdxv1.MarketVector` — a generated protobuf type — directly through `reader`, `router` and `server` (`reader.go:52`, `router.go:181`). The wire type *is* the internal type. This must be decoupled: an internal `IO_Vector` struct with explicit encode/decode at the edges. |
| **Processing messages** | Stage-to-stage payloads must not carry transport metadata. | Currently fine — mathematics consumes `NormalizedObservation`, never a protobuf message. Preserve this. |
| **State identity** | `channel_id` must be the single partition key everywhere, opaque and transport-neutral. | Partially. `entity_id` serves this role but is upper-cased and validated against a fixed supported set (`server.go:167`, `router.go:96`). Case folding must go — it is a financial-symbol convention that would corrupt arbitrary identifiers. |
| **Lineage** | Must be carried in the vector, not inferred from transport ordering or offsets. | **Gap.** No vector-level lineage today; pricing outputs correlate only by `(symbol, timestamp)`. If lineage were later derived from Event Hubs offsets, the system would become transport-dependent. Lineage must be intrinsic. |
| **Partition key** | Must be an explicit `IO_Vector` field, computed once at ingress, used identically by local fan-out and by Event Hubs partitioning. | Implicit today (`entity_id` doubles as identity and key). Separate them. |
| **Scientific invocation interfaces** | Mathematical functions must take plain values/structs and return plain values/structs — no channels, no context, no transport types in signatures. | **Already true and worth protecting.** Every scientific function in SADE takes scalars or plain arrays. `EmissionPolicy.emit` and `PriceCockpitInterpreter.observe` are pure state transitions. `solve_cover` is pure. This is the property that makes the whole migration tractable, and it should be an explicit architectural invariant in `SADE_Go`. |
| **Ordering guarantee** | Must be stated as "per `channel_id`", matching both single-goroutine ownership locally and Event Hubs per-partition ordering. | Consistent with both. |
| **Configuration** | Source and channel configuration must be runtime data, not compile-time constants. | **Gap** — see K.6. |

## K.6 Hard-coding audit

Mandatory section. Every hard-coded value found in production-relevant code,
classified into the four required categories.

### VALID SCIENTIFIC CONSTANT

These encode frozen scientific calibration. They should remain in code, and
changing them changes the science.

| Value | Location | Meaning |
|---|---|---|
| `CONTEXT_LENGTH = 15` | `emitter.py:64` | rolling adaptive context length; gates INITIALIZING→ACTIONABLE |
| `epsilon = 1e-8` | `config.py:123` | numerical guard; `sqrt` of it is the perturbation materiality floor |
| velocity/acceleration/curvature bounds 50 / 200 / 200 | `config.py:17–19` | kinematic clipping bounds |
| `dt_floor = 1e-6` | `config.py:16` | division guard |
| reference `alpha = 0.05`, `min_scale = 1e-4` | `config.py:10–11` | EWMA reference smoothing |
| strength coefficients (6 values) | `config.py:39–48` | |
| coherence channel weights (4 values) | `config.py:54–56` | |
| persistence `alpha = 0.2` | `config.py:61` | |
| perturbation thresholds, `structural_quality_floor = 0.5`, multiplier bounds `(0.8, 1.5)` | `config.py:67–71` | |
| uncertainty coefficients (5 values) | `config.py:76–84` | |
| reversal coefficients (5 values) | `config.py:90–98` | |
| half-life baseline 120, min 15, max 900, multiplier bounds | `config.py:104–109` | |
| forward interval 10 / 60 / 600, `sample_count = 8`, `sampling_exponent = 1.8` | `config.py:114–118` | |
| adaptation learning rates and `ref_alpha` bounds `(0.001, 0.2)` | `config.py:24–27` | |
| magic literals inside formulas: `0.95`/`0.05` (reference.py:14), `0.25` (persistence.py:10), `0.4` (persistence.py:9), `−1.0` (uncertainty.py:28), `−1.2` (reversal.py:31), `/4.0` (reversal.py:23), `0.2`/`0.35`/`0.75` (half_life.py:7–15), `0.1` (adaptation.py:21), `0.7`/`0.6`/`1.1`/`0.8`/`0.35` (forward.py:14), `/10.0` (volume.py:12), `0.15`/`0.1` (model.py:275, 284), `5.0` (model.py:73, 110) | various | **Undocumented inline scientific constants.** They are valid, but they are *not* in `config.py`, so they are invisible to configuration review. Flagged for human attention: a Go port must transcribe each one exactly. |
| `derivative_window = 15`, `f4_window = 30` | `pipeline.py:106–107` | scientific window lengths |
| `epsilon = 0.0035332071428566536` | `pipeline.py:108` | Price epsilon; a fitted/derived value, not a round number |
| `ridge_lambda = 1.0`, `rtol = 1e-6` | `pipeline.py:109–110` | |
| ridge matrix `diag([0,1,1,1])` | `dynamics.py:88` | intercept deliberately unpenalised |
| RK45 horizon `(0.0, 1.0)`, `linspace(0,1,11)` | `projection.py:78, 108` | one-minute projection with 11 dense points |
| instability factor `1e6` | `projection.py:120` | |
| `atol` cap factor `0.1` | `projection.py:101` | |
| `directional_epsilon = 1e-15` | `perturbation.py:28` | |
| companion matrix structure `[[0,1,0],[0,0,1],c]` | `numerical.py:87` | |
| `PolicyConfig` thresholds: condition median 7.8357…, q95 13.0403…, eigenvalue median 0.4221…, q95 0.6449…, amplification median 2.2423…, q95 2.6637… | `pipeline.py:165–171` | **Calibrated percentiles from a prior study.** These are the values most exposed to numerical drift (Part F.2.3). They are hard-coded as pipeline defaults rather than declared calibration artifacts. Flagged for human review. |
| `CockpitPolicyConfig`: `zero_proximity_threshold = 0.9`, `deceleration_strength_threshold = 0.05`, `persistence_observations = 1`, `candidate_hold_observations = 0` | `pipeline.py:182–188` | |
| D02/D04 validation bounds (10–600, 15–900, 0–1, ±50) | `builder.py`, `models.py` | duplicated from config; defence in depth |
| `uncertainty = 0.15`, `reversal_propensity = 0.1`, `decay_relevance = 1.0` initial values | `state.py:15–17` | initial scientific state |
| `HalfLifeState(120.0, 120.0)` default | `state.py:41` | |
| `BASELINE_RULE_FINGERPRINT`, `BASELINE_IMPLEMENTATION_FINGERPRINT` | `scientific_baseline.py:38–39` | provenance identities |

### CONFIGURATION DEFAULT

Reasonable defaults with an override path. Acceptable, though several should
become explicit configuration.

| Value | Location | Override available? |
|---|---|---|
| `defaultPort = "50051"` | `main.go:21` | Yes — `GRPC_PORT` |
| CSV source paths | `main.go:31–32` | Yes — `SDX_<ENTITY>_CSV` per entity |
| `DEFAULT_ENDPOINT = "localhost:50051"` | `sdx_client.py:59` | Yes — constructor / CLI `--endpoint` |
| `max_vectors = 100` | `pipeline.py:192`, `__main__.py:50` | Yes — CLI |
| `timeout_seconds = 60.0` | `pipeline.py:193` | Yes — CLI |
| `DEFAULT_UNIT_RUN_OUTPUT_DIR` | `pipeline.py:55` | Yes — CLI |
| `enable_cockpit = True` | `pipeline.py:113` | Yes — config field |
| `default_session = "UNKNOWN"`, `default_source_provider = "SDX_V1_1_STREAM"` | `pipeline.py:111–112` | Yes — config field |
| client `--timeout 30s`, `--max-vectors 100`, `--source-dir` | `cmd/sdx-client/main.go:28–30` | Yes — flags |
| graceful-shutdown timeout `5 * time.Second` | `main.go:76` | No override — minor |

### TEST FIXTURE

Correctly confined to tests, with one exception noted below.

| Value | Location |
|---|---|
| `aaplPath` and `stockPaths` maps | `reader_test.go:17`, `server_test.go:27–35` |
| instrument symbols throughout Go tests | `reader_test.go`, `router_test.go`, `server_test.go` |
| `APTF_ROOT = Path("C:/Users/chino/APTF")` with `pytest.skip` guard | `tests/test_pricing_migration_equivalence.py:64, 78–79` — correctly guarded |
| synthetic fixture series `100 + 0.2i + 1.5 sin(i/5)` | `test_pricing_migration_equivalence.py:101` |
| `FakeClient`, `FakeEmitter`, `_vector`, `_record` helpers | `test_sade_pipeline.py`, `test_pricing_pipeline.py` |
| hard-coded expected values (physical_row 2/101, counts) | `test_sade_pipeline.py:132–134` |

### INVALID RUNTIME HARD-CODING

These must be removed before scale testing. Each is a genuine blocker.

| # | Value | Location | Why invalid | Blocking |
|---:|---|---|---|---|
| 1 | `var entities = []string{"AAPL","AMZN","META","MSFT","TSLA"}` | `cmd/sdx-server/main.go:23` | The **set of channels the runtime can serve is fixed at compile time**. Adding a source requires recompiling the binary. Directly contradicts N-source ingestion and thousands-of-channel scale. Per-entity env overrides exist for *paths* but cannot extend the *set*. | **YES — hardest blocker** |
| 2 | `"data_sources/Stocks/" + entity + "_1min_firstratedata.csv"` | `cmd/sdx-server/main.go:31` | Provider-specific filename convention and a fixed relative directory compiled into the runtime. Encodes one provider's naming into the server. | **YES** |
| 3 | `expectedHeader = []string{"timestamp","open","high","low","close","volume"}` | `internal/reader/reader.go:16` | Single hard-coded source schema in the runtime reader. Any other source shape fails header validation. Blocks generic payloads. | **YES** |
| 4 | `DefaultCapacity = 10` | `internal/router/router.go:15` | Backpressure queue depth is a compile-time constant with no configuration path — the primary tuning knob for a thousands-channel runtime. | **YES** |
| 5 | Unbuffered shared fan-in channel | `internal/router/router.go:183` | Structural, not a literal, but hard-coded by construction: `make(chan *sdxv1.MarketVector)` with no capacity couples all channels through one send loop. | **YES** |
| 6 | Shared `cancel()` on any producer error | `internal/router/router.go:242` | Hard-codes cross-channel failure coupling. One bad source kills all channels. | **YES** |
| 7 | `entity: str = "AAPL"` as a **production config default** | `adaptive_pipeline/pipeline.py:191` | A named financial instrument is the default value of a production runtime configuration field — not a test file. Also `entity="AAPL"` hard-coded in `__main__.py:49`. | **YES** |
| 8 | `entity="AAPL"`, `max_vectors=100`, `entities=["AAPL"]` inside the unit-run harness | `unit_run/run_pricing_001.py:102,110,116`; `run_001.py:86` | Test/validation assumptions in `sade/` (the shipped package) rather than in `tests/`. Leaks a fixture into the product. | Medium |
| 9 | `APTF_ROOT = Path("C:/Users/chino/APTF")` in **production package** | `unit_run/run_001.py:45`; `run_pricing_001.py:51` | An absolute developer-machine path compiled into a shipped module. Only used for an independence audit, but it is still a machine-specific path in `sade/`. | Medium |
| 10 | `data_valid = "true"` and `session_type = "UNKNOWN"` injected as facts | `adaptive_pipeline/pipeline.py:105–106` | Fabricated input values presented to the scientific path as observations. Honestly declared in the summary as SADE assumptions, but `data_valid="true"` becomes `source_quality = 1.0` (`normalizer.py:73–74`), which feeds perturbation classification (`model.py:131`) and uncertainty (`model.py:181`). **A fabricated constant is influencing scientific output.** | **YES — scientific integrity** |
| 11 | `receive_time = event_time` | `adaptive_emitter/normalizer.py:68` | Fabricates the ingress timestamp as equal to the source timestamp. Makes latency unmeasurable in principle and silently asserts zero ingestion delay. | **YES — blocks latency measurement** |
| 12 | `physical_row = source_row_index + 2` | `adaptive_pipeline/pipeline.py:67`, consumed at `emitter.py:190, 227` | A legacy CSV row-numbering convention (header + 1-based) embedded in the runtime **and in the `observation_id` hash**. Round-trips to nothing functionally, but permanently couples scientific identity to a file-format artifact. | **YES — identity coupling** |
| 13 | `strings.ToUpper` on channel identifiers | `internal/server/server.go:167`; `internal/router/router.go:96` | Financial-symbol case convention applied to what should be opaque channel identifiers. Would corrupt or collide arbitrary `IO_Vector` IDs. | **YES** |
| 14 | Single-stream assumption throughout SADE | `AdaptivePipelineConfig.entity: str`, `PricingPipelineConfig.entity: str`, all implicitly-keyed state (J.1) | The most pervasive hard-coded assumption: exactly one channel per process. Not a literal, but the deepest structural constraint. | **YES** |
| 15 | `insecure.NewCredentials()` / `grpc.insecure_channel` | `cmd/sdx-client/main.go:37`; `sdx_client.py:96` | Transport security hard-coded off with no configuration path. Not a scale blocker; an Azure blocker. | Azure |
| 16 | Dead-but-broken `time_term` branch | `projection.py:93–94` vs `dynamics.py:41–52` | `time_term=True` would `KeyError` on `fit["time_mean"]` because `allocate_fit` never creates it. Latent trap. | Low |

Summary: **10 items are true scale blockers**, of which items 10, 11 and 12
additionally touch scientific integrity or identity and therefore need human
scientific review, not just engineering cleanup.

---

# Part L — Classification and Migration Matrices

## L.1 Go refactorability classification

| Class | Definition | Modules | Lines | Share |
|---|---|---:|---:|---:|
| **G0** | Already implemented in Go | 5 handwritten SDX files (+2 generated) | 833 | — |
| **G1** | Trivial Go refactor: mapping, control, validation, serialization, simple state. No floating-point mathematics on the scientific path. | 34 | 2,502 | 53.9% of SADE Python |
| **G2** | Straightforward Go mathematics: scientific logic with a direct, low-risk Go equivalent using only `math` stdlib. | 20 | 1,650 | 35.5% |
| **G3** | Go mathematics requiring rigorous equivalence validation: dense linear algebra with library-algorithm sensitivity. | 3 | 345 | 7.4% |
| **P1** | Retain Python initially: strong current reason. | 1 | 146 | 3.1% |
| **P2** | Insufficient evidence. | **0** | **0** | **0.0%** |

That P2 is empty is itself a finding: both codebases were small enough to read
completely, so no module's classification rests on inference.

**G0 evidence:** ingestion, vector construction, partitioning, per-channel
ordering, bounded-queue backpressure, fan-out/fan-in, gRPC transport, status,
cancellation and shutdown all execute in Go today
(`reader.go`, `router.go`, `server.go`, `main.go`).

**G1 evidence:** every G1 module was read and confirmed to contain no
floating-point scientific computation — only dict/struct mapping, presence and
bound checks, JSON/CSV serialisation, counter aggregation, configuration
dataclasses, transport calls, and CLI wiring. Representative: `pipeline.py`
`build_source_row`/`_build_record`/`_write_csv`; `config.py`'s 15 frozen
dataclasses; `contracts.py`'s two dataclasses with `as_dict`; `sdx_client.py`
entirely.

**G2 evidence:** every G2 function was read and confirmed to use only stdlib
`math`, comparisons, and `statistics.median`. The complete import scan over
`sade/d01/`, `sade/d02/`, `sade/d04/` and `price_engine/` returns zero NumPy and
zero SciPy. Go's `math` package covers `Exp`, `Sqrt`, `Log1p`, `Pow`,
`IsNaN`/`IsInf`.

**G3 evidence:** exactly three modules perform dense linear algebra —
`derivatives.py` (`lstsq`), `dynamics.py` (`solve`, `cond`, population `std`),
`numerical.py` (`eigvals`, `expm`). All three have gonum equivalents present in
the local module cache, and all three feed discrete classification thresholds,
which is precisely why they need validation rather than assumption.

**P1 evidence:** `projection.py` — `scipy.integrate.solve_ivp` has no equivalent
anywhere in the local Go dependency set (verified against the extracted gonum
tree and the full module graph).

## L.2 Module-by-module migration matrix

| Repo | Current module | Function/class | Lang | Responsibility | Dependencies | State | Class | Proposed owner | Evidence |
|---|---|---|---|---|---|---|---|---|---|
| SDX | `cmd/sdx-server/main.go` | `main` | Go | bootstrap, listener, lifecycle | grpc | none | G0 | Go runtime (generalise config) | reads compile-time entity slice L23 |
| SDX | `cmd/sdx-client/main.go` | `main` | Go | fidelity validation client | grpc | none | G0 | Go test tooling | asserts source order + values |
| SDX | `internal/reader/reader.go` | `SDReader.Read` | Go | CSV source read, vector construction | stdlib csv | `status` (RWMutex) | G0 | Go `ingress/source` adapter | fixed header L16; blocking send L85 |
| SDX | `internal/reader/reader.go` | `parseVector` | Go | field parse | stdlib | none | G0 | Go | L130–159 |
| SDX | `internal/router/router.go` | `RoutePartitions` | Go | fan-out, per-partition queues | stdlib | `statuses` | G0 | Go `ingress/partition` | L212–270 |
| SDX | `internal/router/router.go` | `Route` | Go | fan-in | stdlib | counters | G0 | Go, **rework** | unbuffered merge L183 |
| SDX | `internal/router/router.go` | `Configure`, `Statuses`, `SourceStatuses` | Go | control + observability | stdlib | `configured` | G0 | Go | L88–179 |
| SDX | `internal/server/server.go` | `StreamVectors` | Go | gRPC streaming | grpc | none | G0 | Go (internal call after consolidation) | L27–55 |
| SDX | `internal/server/server.go` | `ConfigureRouter`, `Get*Status` | Go | control RPCs | grpc | none | G0 | Go | L57–118 |
| SDX | `internal/server/server.go` | `validateRequest` | Go | validation | grpc | none | G0 | Go, **remove ToUpper** | L167 |
| SDX | `gen/sdx/v1/*` | generated | Go | protobuf/gRPC bindings | protobuf | none | G0 | regenerate for `IO_Vector` | 1,122 lines |
| SADE | `sade/__init__.py` | metadata | Py | version | none | none | G1 | Go | 34 lines |
| SADE | `sade/__main__.py` | `main`, `_parse_args` | Py | CLI | argparse | none | G1 | Go | `entity="AAPL"` default L49 |
| SADE | `adaptive_pipeline/pipeline.py` | `physical_row_from_source_index` | Py | legacy row mapping | none | none | G1 | Go — **flag: identity coupling** | `+2` L67 |
| SADE | `adaptive_pipeline/pipeline.py` | `build_source_row` | Py | field mapping | none | none | G1 | Go — **flag: fabricated fields** | L105–106 |
| SADE | `adaptive_pipeline/pipeline.py` | `_validate_vector` | Py | entity + order validation | none | none | G1 | Go | L110–132 |
| SADE | `adaptive_pipeline/pipeline.py` | `_build_record` | Py | flatten for CSV | none | none | G1 | Go | L135–183 |
| SADE | `adaptive_pipeline/pipeline.py` | `AdaptivePipelineConfig` | Py | config | dataclass | none | G1 | Go | `entity` default `"AAPL"` |
| SADE | `adaptive_pipeline/pipeline.py` | `AdaptivePipeline.process_vector` | Py | orchestration | none | `_expected_index`, `_rows` | G1 | Go owner goroutine | `_rows` unbounded |
| SADE | `adaptive_pipeline/pipeline.py` | `AdaptivePipeline.run` | Py | stream loop + aggregation | grpc | counters | G1 | Go | single-threaded loop L300 |
| SADE | `adaptive_pipeline/pipeline.py` | `_write_csv`, `close` | Py | serialization, lifecycle | csv | none | G1 | Go | L383–432 |
| SADE | `adaptive_emitter/normalizer.py` | `source_row_to_normalized_observation` | Py | mapping | datetime | `entity_id` | G1 | Go — **flag: `receive_time` fabricated** | L68 |
| SADE | `adaptive_emitter/emitter.py` | `canonical_sha256` | Py | deterministic hashing | json, hashlib | none | G1 | Go — **flag: repr equivalence** | L69–72 |
| SADE | `adaptive_emitter/emitter.py` | `DevelopmentObservationStream` | Py | historical dev reader | csv | file handle | G1 | **drop** — unused | L75–121 |
| SADE | `adaptive_emitter/emitter.py` | `_adaptive_properties` | Py | rolling-15 statistics | statistics | reads context | G2 | Go | L151–165 |
| SADE | `adaptive_emitter/emitter.py` | `_decide` | Py | decision predicate | none | none | G2 | Go | L168–174 |
| SADE | `adaptive_emitter/emitter.py` | `AdaptiveEmitter.process` | Py | scientific sequencing + state | stdlib | context, position, audits | G2 | Go owner goroutine | L176–370 |
| SADE | `configuration/scientific_baseline.py` | `get_baseline_fingerprints` | Py | provenance constants | none | none | G1 | Go | L38–53 |
| SADE | `input/sdx_client.py` | `SadeSdxClient` (all) | Py | gRPC transport | grpcio | channel, stubs | G1 | **eliminate** (in-process) | L76–172 |
| SADE | `input/generated/sdx/v1/*` | generated | Py | protobuf bindings | grpcio | none | G1 | **eliminate** | 285 lines |
| SADE | `d01/v02/config.py` | 15 dataclasses | Py | configuration + `sha256` | json, hashlib | immutable | G1 | Go | 130 lines |
| SADE | `d01/v02/state.py` | `StateVector`, `HalfLifeState`, `RuntimeState` | Py | scientific state | dataclass | **core recursive** | G1 | Go per-channel struct | L1–47 |
| SADE | `d01/v02/observations.py` | `NormalizedObservation` | Py | input mapping | dataclass | immutable | G1 | Go | L6–42 |
| SADE | `d01/v02/observations.py` | `assert_causal_sequence` | Py | causal validation | none | none | G1 | Go | L45–51 |
| SADE | `d01/v02/outputs.py` | `DMOOutput`, `FMOOutput`, `FMOSample` | Py | output serialization | dataclass | none | G1 | Go | 51 lines |
| SADE | `d01/v02/snapshot.py` | `to_snapshot`, `from_snapshot`, `state_hash` | Py | state serialization | json, hashlib | none | G1 | Go | 32 lines |
| SADE | `d01/v02/trace.py` | `TraceRecord` | Py | diagnostics | dataclass | none | G1 | Go telemetry — **do not accumulate** | `model.py:333` |
| SADE | `d01/v02/reference.py` | `update_reference_and_scale` | Py | EWMA reference/scale | stdlib | 2 scalars | G2 | Go | 12 lines |
| SADE | `d01/v02/kinematics.py` | `compute_kinematics`, `_clip` | Py | level/vel/accel/curvature | stdlib | none | G2 | Go | 27 lines |
| SADE | `d01/v02/innovation.py` | `innovation_magnitude` | Py | residual + magnitude | `math.sqrt` | none | G2 | Go | 7 lines |
| SADE | `d01/v02/volume.py` | `update_volume_influence` | Py | volume influence | `math.log1p` | 1 scalar | G2 | Go | 11 lines |
| SADE | `d01/v02/coherence.py` | `compute_coherence` | Py | weighted coherence | stdlib | none | G2 | Go | 11 lines |
| SADE | `d01/v02/strength.py` | `compute_strength` | Py | sigmoid strength | `math.exp` | none | G2 | Go | 25 lines |
| SADE | `d01/v02/persistence.py` | `update_persistence` | Py | EWMA persistence | stdlib | 1 scalar | G2 | Go | 10 lines |
| SADE | `d01/v02/uncertainty.py` | `compute_uncertainty` | Py | sigmoid uncertainty | `math.exp` | none | G2 | Go | 24 lines |
| SADE | `d01/v02/reversal.py` | `compute_reversal_propensity` | Py | sigmoid reversal | `math.exp` | none | G2 | Go | 29 lines |
| SADE | `d01/v02/perturbation.py` | `classify_perturbation`, `infer_perturbation_class`, `_direction` | Py | perturbation classification | `math.sqrt` | none | G2 | Go | 65 lines |
| SADE | `d01/v02/half_life.py` | `adapt_half_life` | Py | half-life adaptation | stdlib | 2 scalars | G2 | Go | 13 lines |
| SADE | `d01/v02/adaptation.py` | `update_parameters` | Py | bounded parameter adaptation | stdlib | param dict | G2 | Go | 25 lines |
| SADE | `d01/v02/forward.py` | `compute_forward_interval`, `forward_samples`, `propagate_level` | Py | elastic forward projection | `**` | none | G2 | Go | 22 lines |
| SADE | `d01/v02/health.py` | `evaluate_health` | Py | health classification | `math.isfinite` | mutates counters | G2 | Go | 26 lines |
| SADE | `d01/v02/model.py` | `D01V02Model.step` | Py | 22-step scientific sequencing | stdlib | `RuntimeState` | G2 | Go owner goroutine | L59–360 |
| SADE | `d01/v02/model.py` | `_state_hash`, `snapshot` | Py | state hashing | json, hashlib | none | G1 | Go | L43–57, 362 |
| SADE | `d02/v02/models.py` | `ReturnShape`, `ForwardSample`, `PathDirection` | Py | validated shape | `math.isfinite` | immutable | G1 | Go | 85 lines |
| SADE | `d02/v02/builder.py` | `build_return_shape` | Py | shape geometry | `**` | none | G2 | Go | L63–108 |
| SADE | `d02/v02/builder.py` | `_validate_input`, `_require_*` | Py | validation | stdlib | none | G1 | Go | L9–60 |
| SADE | `d04/envelope/capturability_model.py` | `geometry/structural/risk_quality`, `evaluate` | Py | capturability mathematics | `math.sqrt`, `**` | none | G2 | Go | L43–91 |
| SADE | `d04/envelope/capturability_model.py` | `validate_return_shape` | Py | validation | stdlib | none | G1 | Go — **flag: exact float equality** | L23–41 |
| SADE | `d04/models/envelope_context.py` | `EnvelopeContext` | Py | context + provenance invariant | **pydantic** | immutable | G1 | Go struct + `Validate()` | 70 lines |
| SADE | `d04/models/capturability.py` | `CapturabilityResult` | Py | result + bounds | **pydantic** | none | G1 | Go | 11 lines |
| SADE | `d04/models/enums.py` | 6 enums | Py | vocabulary | enum | none | G1 | Go (4 unused) | 36 lines |
| SADE | `pricing_pipeline/pipeline.py` | `PricingPipeline.process` | Py | pricing orchestration | **numpy** | 8 unbounded lists, policy/cockpit state | G1 | Go owner goroutine — **fix O(n²), bound history** | L219–362 |
| SADE | `pricing_pipeline/pipeline.py` | `PricingPipelineConfig`, `_step_result`, `_to_minutes`, `summary` | Py | config, aggregation | dataclass | counters | G1 | Go | L83–128, 364–421 |
| SADE | `pricing_pipeline/derivatives.py` | `causal_quadratic` | Py | causal quadratic fit | **numpy lstsq** | none | **G3** | Go + `mat.SVD` | L44–95 |
| SADE | `pricing_pipeline/derivatives.py` | `derivative_state` | Py | derivative classification | stdlib | none | G2 | Go — **unused today** | L98–131 |
| SADE | `pricing_pipeline/dynamics.py` | `fit_f4` | Py | ridge F4 fit | **numpy solve/cond/std** | none | **G3** | Go + `mat.Solve`/`mat.Cond` | L55–111 |
| SADE | `pricing_pipeline/dynamics.py` | `allocate_fit`, `valid_fit` | Py | allocation, finiteness | numpy | none | G1 | Go | L41–52, 114–117 |
| SADE | `pricing_pipeline/projection.py` | `solve_cover` — RK45 core | Py | ODE integration | **scipy solve_ivp** | none | **P1** | **Python initially**, Go later | L106–114 |
| SADE | `pricing_pipeline/projection.py` | instability, `D_local_maximum`, envelope exit | Py | post-integration diagnostics | numpy | none | G1 | **Go now** — shrinks the bridge | L120–137 |
| SADE | `pricing_pipeline/numerical.py` | `build_numerical_row` — eigen + expm | Py | stability metrics | **numpy eigvals, scipy expm** | none | **G3** | Go + `mat.Eigen`/`mat.Dense.Exp` | L86–89 |
| SADE | `pricing_pipeline/numerical.py` | payload assembly | Py | dict assembly | numpy, json | none | G1 | Go | L100–129 |
| SADE | `price_engine/contracts.py` | `MarketObservation`, `PriceEmission` | Py | contracts | dataclass | immutable | G1 | Go — **flag: no lineage** | 171 lines |
| SADE | `price_engine/engine.py` | `PriceEngine.observe` | Py | coherence gate | stdlib | none | G1 | Go | L73–111 |
| SADE | `price_engine/policy.py` | `EmissionPolicy.emit`, `_build` | Py | policy classification | `math.isfinite` | `PolicyState` (external) | G2 | Go — **flag: ordered dedupe** | L144–285 |
| SADE | `price_engine/policy.py` | `_direction`, `_acceleration`, `_phase`, `_turning_tendency` | Py | categorical mapping | stdlib | none | G2 | Go | L92–135 |
| SADE | `price_engine/policy.py` | `PolicyConfig`, `PolicyState` | Py | config + state | dataclass | 2 fields | G1 | Go | L47–89 |
| SADE | `price_engine/cockpit.py` | `PriceCockpitInterpreter.observe`, `_motion_state` | Py | cockpit interpretation | `math.isfinite` | `CockpitState` (external) | G2 | Go | L110–254 |
| SADE | `price_engine/cockpit.py` | configs, emission dataclass | Py | config + contract | dataclass | 6 fields | G1 | Go | L45–89 |
| SADE | `unit_run/run_001.py` | `run_unit_001`, audit hook | Py | validation harness | stdlib | audit lists | G1 | Go test tooling | `APTF_ROOT` L45 |
| SADE | `unit_run/run_pricing_001.py` | `run_pricing_unit_001` | Py | integrated harness | stdlib | in-memory rows | G1 | Go test tooling | `entity="AAPL"` L102 |

## L.3 Mathematics migration matrix

| Mathematical operation | Current implementation | Library | Go feasibility | Numerical risk | Proposed phase |
|---|---|---|---|---|---|
| Adaptive reference/scale EWMA | `reference.py:12–14` | stdlib | Direct | LOW (recursive accumulation) | 3 |
| Level / velocity / acceleration / curvature | `kinematics.py:24–32` | stdlib + `**1.5` | Direct (`math.Pow`) | LOW | 3 |
| Innovation residual + magnitude | `innovation.py:7–10` | `math.sqrt` | Direct (exact `sqrt`) | **NONE** | 3 |
| Volume influence | `volume.py:9–15` | `math.log1p` | Direct (`math.Log1p`) | LOW | 3 |
| Coherence | `coherence.py:4–13` | stdlib | Direct | **NONE** | 3 |
| Strength sigmoid | `strength.py:8–31` | `math.exp` | Direct (`math.Exp`) | LOW (1–2 ulp) | 3 |
| Persistence EWMA | `persistence.py:6–13` | stdlib | Direct | LOW | 3 |
| Uncertainty sigmoid | `uncertainty.py:8–30` | `math.exp` | Direct | LOW | 3 |
| Reversal sigmoid | `reversal.py:8–35` | `math.exp` | Direct | LOW | 3 |
| Perturbation classification | `perturbation.py:15–81` | `math.sqrt` | Direct | **NONE** (comparisons) | 3 |
| Half-life adaptation | `half_life.py:6–16` | stdlib | Direct | LOW | 3 |
| Bounded parameter adaptation | `adaptation.py:6–28` | stdlib | Direct; **iterate a fixed key order** | LOW | 3 |
| Forward interval + `τ` samples | `forward.py:6–29` | `**` | Direct (`math.Pow`) | LOW | 3 |
| FMO decay `2^(−τ/hl)` | `model.py:274` | `**` | Direct | LOW | 3 |
| Level propagation | `forward.py:28–29` | stdlib | Direct | **NONE** | 3 |
| Health classification | `health.py:8–30` | `math.isfinite` | Direct | **NONE** | 3 |
| D01 state hashing | `model.py:43–57` | json+hashlib | Direct **if** repr matched | see risk R14 | 2 |
| D02 terminal / max displacement | `builder.py:78–81` | stdlib | Direct | **NONE** | 3 |
| D02 path direction | `builder.py:82–87` | stdlib | Direct | **NONE** | 3 |
| D02 terminal decay factor | `builder.py:88` | `**` | Direct | LOW | 3 |
| D04 geometry quality | `capturability_model.py:44–48` | stdlib | Direct | **NONE** | 3 |
| D04 structural quality `(·)^(1/3)` | `capturability_model.py:51–52` | `**(1/3)` | Direct — **must use `Pow(x,1.0/3.0)`, NOT `Cbrt`** | LOW | 3 |
| D04 risk quality | `capturability_model.py:55–56` | `math.sqrt` | Direct | **NONE** | 3 |
| D04 capturability + eligibility | `capturability_model.py:58–91` | stdlib | Direct | **NONE** | 3 |
| Rolling-15 median/min/max/range/counts | `emitter.py:151–165` | `statistics.median` | Direct (sort-based) | **NONE** | 3 |
| Adaptive decision predicate | `emitter.py:168–174` | stdlib | Direct | **NONE** | 3 |
| State support ratio | `model.py:291–294` | stdlib | Direct | LOW | 3 |
| **Causal quadratic least squares** | `derivatives.py:86` | **`np.linalg.lstsq`** | `mat.SVD`; must match rank criterion | **MEDIUM** | 4 |
| Derivative-state classification | `derivatives.py:98–131` | stdlib | Direct (currently unused) | **NONE** | 3 |
| `jp` discrete difference | `pipeline.py:282–285` | numpy | Direct | LOW (differencing amplifies) | 4 |
| **Window mean** | `dynamics.py:94` | `np.mean(axis=0)` | Direct loop | LOW | 4 |
| **Window std (population)** | `dynamics.py:95` | `np.std(axis=0)` **ddof=0** | Explicit loop — **`stat.StdDev` is WRONG (Bessel)** | **HIGH if mistranslated** | 4 |
| **Ridge normal-equation solve** | `dynamics.py:100` | **`np.linalg.solve`** | `mat.Dense.Solve` (LU) | **HIGH** (squared condition) | 4 |
| **Condition number (2-norm)** | `dynamics.py:110` | **`np.linalg.cond`** | `mat.Cond(a, 2)` — **must pass norm 2** | **HIGH** (feeds discrete thresholds) | 4 |
| Coefficient de-standardisation | `dynamics.py:103–105` | numpy | Direct | LOW | 4 |
| Window min/max envelope | `dynamics.py:108–109` | numpy | Direct | **NONE** | 4 |
| **RK45 initial-value integration** | `projection.py:106–114` | **`scipy.integrate.solve_ivp`** | **NO Go library — custom Dormand–Prince required** | **MEDIUM-HIGH** | 5 |
| RK45 vector field (affine jerk) | `projection.py:89–95` | numpy | Direct | LOW | 5 |
| Per-component `atol` construction | `projection.py:97–103` | numpy | Direct | LOW — **must be per-component** | 5 |
| Instability rejection | `projection.py:120–122` | numpy | Direct | **NONE** | 4 |
| `D_local_maximum` | `projection.py:123` | `np.linalg.norm` | `floats.Norm` | LOW | 4 |
| Envelope-exit detection | `projection.py:124–137` | numpy masking | Direct loops | **NONE** | 4 |
| **Companion-matrix eigenvalues** | `numerical.py:88` | **`np.linalg.eigvals`** | `mat.Eigen` | **MEDIUM-HIGH** (sign near zero flips stability) | 4 |
| **Matrix exponential amplification** | `numerical.py:89` | **`scipy.linalg.expm`** | `mat.Dense.Exp` (same Higham family) | **MEDIUM-HIGH** | 4 |
| Policy phase / tendency / direction / acceleration | `policy.py:92–135` | stdlib | Direct | **NONE** | 2 |
| Policy confidence tiering | `policy.py:193–202` | stdlib | Direct | **NONE** itself; inherits G3 inputs | 2 |
| Policy stability + colour + debounce | `policy.py:204–226` | stdlib | Direct; **ordered dedupe needed** | **NONE** | 2 |
| Cockpit zero-proximity + deceleration | `cockpit.py:133–140` | stdlib | Direct | LOW | 2 |
| Cockpit persistence / candidate / hysteresis | `cockpit.py:142–211` | stdlib | Direct | **NONE** | 2 |
| Canonical SHA-256 identity | `emitter.py:69–72` | json+hashlib | Direct **if** float repr matched | see risk R14 | 2 |

## L.4 Runtime responsibility matrix

| Responsibility | Current owner | Current language | Proposed SADE_Go owner | Migration required? |
|---|---|---|---|---:|
| Source ingestion | `SDReader.Read` | Go | Go `ingress/source` adapters (generalise beyond CSV) | Generalise |
| Source adapters (N sources) | none — CSV only | Go | Go `SourceAdapter` interface | **YES** |
| Normalisation | `reader.parseVector` + `SourceRowNormalizer` (split across languages) | Go + Python | Go ingress, single place | **YES** |
| Vector construction | `parseVector` → `MarketVector` | Go | Go `IO_Vector` (generic payload) | Generalise |
| Ingress timestamping | **does not exist**; `receive_time` fabricated | — | Go ingress | **YES** |
| Sequence assignment | per-request loop counter | Go | Go ingress, durable per channel | **YES** |
| Routing | `router.Route` / `RoutePartitions` | Go | Go partition/ownership registry | Rework fan-in |
| Partitioning | by `entity_id` | Go | by `channel_id` (opaque) | Generalise |
| Ordering | per-partition goroutine (Go) + strict index checks (Python) | Both | Go single-owner goroutine per channel | Consolidate |
| Concurrency | goroutines (ingest only); **none** in processing | Go / none | Go owner goroutine per channel | **YES** |
| Fan-out | `RoutePartitions` | Go | Go, by `channel_id` | Keep |
| Fan-in | `Route` — unbuffered, couples channels | Go | Go, bounded, per-channel egress | **YES — rework** |
| Pub/Sub | none | — | Go local broker; ingress + output only | **YES** |
| Adaptive mathematics (D01/D02/D04) | `sade/d01`,`d02`,`d04`, emitter | Python | **Go** | **YES** (G2) |
| Pricing derivative mathematics | `derivatives.py` | Python (NumPy) | **Go** + `mat.SVD` | **YES** (G3) |
| F4 fitting | `dynamics.py` | Python (NumPy) | **Go** + `mat.Solve`/`mat.Cond` | **YES** (G3) |
| RK45 | `projection.py` | Python (SciPy) | **Python initially**, Go later | Phase 5 |
| Stability metrics (eigen, expm) | `numerical.py` | Python (NumPy/SciPy) | **Go** + `mat.Eigen`/`mat.Dense.Exp` | **YES** (G3) |
| Price policy + cockpit | `policy.py`, `cockpit.py` | Python (stdlib) | **Go** | **YES** (G2) |
| Scientific state | D01 `RuntimeState`, emitter context, policy/cockpit state | Python | **Go**, per-channel, goroutine-local | **YES** |
| Runtime state | split: partition status (Go), counters/ordering (Python) | Both | **Go** | **YES** |
| State bounding | **unbounded** in 8 places | Python | Go ring buffers | **YES — blocker** |
| Backpressure | bounded queues (Go); none in processing | Go | Go per-channel bounded queues | Extend |
| Lifecycle | `signal.NotifyContext` + `GracefulStop` (Go); CLI exit (Python) | Both | **Go** supervisor + per-channel lifecycle | Extend |
| Health | `evaluate_health` (scientific); partition/source status (runtime) | Both | Go: keep both, separated | Migrate |
| Observability / telemetry | log lines + status RPCs; discarded `component_lifecycle_ns` | Both | Go + OpenTelemetry (already in module graph) | **YES** |
| Error control | typed errors + gRPC codes (Go); explicit raises (Python) | Both | **Go**, per-channel scoped | **YES** — remove shared cancel |
| Recovery | none — no retry, no resume, no checkpoint | — | Go: per-channel restart + state snapshot/restore (`to_snapshot`/`from_snapshot` exist as a seed) | **YES** |
| Configuration | env vars (Go); in-code dataclasses (Python) | Both | Go: runtime config for operational values; frozen constants stay in code | **YES** |
| Serialization | protobuf (Go); CSV/JSON/SHA-256 (Python) | Both | Go, boundary-only | **YES** |
| Output construction | `_build_record`, `as_dict` | Python | Go `IO_Vector(OUTPUT)` | **YES** |
| Lineage | partial (adaptive only); none for pricing | Python | Go, intrinsic to `IO_Vector` | **YES** |
| Result routing | direct function call in a script | Python | Go output channels + Pub/Sub | **YES** |
| Decision processing | **not implemented** | — | Go, future (out of scope) | Deferred |
| Order issue/control | **not implemented** | — | Go, future (out of scope) | Deferred |

---

# Part M — Target Architecture Diagrams

## DIAGRAM 3 — PROPOSED SADE_GO LOCAL RUNTIME

The task requires the target diagram verbatim as the starting reference:

```text
                    SADE_Go runtime

IO_Vector
   ↓
Ingress channel
   ↓
Partition / ownership logic
   ↓
Pub/Sub topics or typed channels
   ↓
┌──────────────────────────────────────┐
│ Go processing graph                  │
│                                      │
│ adaptive Go routines/services        │
│ pricing Go routines/services         │
│ state management                     │
│ routing                              │
│ fan-out / fan-in                     │
└──────────────────┬───────────────────┘
                   │
                   │ only for math still in Python
                   ▼
          Python computation service
                   │
                   ▼
          result channel / topic
                   │
                   ▼
             Go processing resumes
                   │
                   ▼
              Output IO_Vector
```

### FORENSICALLY REFINED VERSION

Refined against actual repository evidence. Six changes, each justified.

```text
                        SADE_Go runtime  (ONE Go process)

SOURCE_000001 ─┐
SOURCE_000002 ─┤  SourceAdapter interface; 1 goroutine per active source
    ...        │  (generalises SDReader; CSV becomes one adapter)
SOURCE_00000N ─┘
      │
      │  normalise -> IO_Vector(INPUT)
      │  assign io_vector_id, channel_id, sequence, ingest_timestamp
      │  [FIXES: fabricated receive_time; per-request sequence restart]
      ▼
Ingress channel          chan IO_Vector   (bounded; capacity CONFIGURABLE)
      │                                   [FIXES: DefaultCapacity=10 compile-time]
      ▼
Partition / ownership logic
      │  owner = registry[channel_id]; create on first sight; retire on idle
      │  FAN-OUT strictly by channel_id -- never round-robin
      │  [FIXES: compile-time entity slice; ToUpper on identifiers]
      ▼
Typed Go channels, one per IO_Vector channel   (bounded)
      │  NOT Pub/Sub here: single consumer per channel, so a topic adds nothing
      ▼
┌───────────────────────────────────────────────────────────────────────────┐
│ Go processing graph -- ONE OWNER GOROUTINE PER IO_VECTOR CHANNEL          │
│                                                                           │
│  Per-channel state, goroutine-local, NO LOCKS, ordering by construction:  │
│    D01 RuntimeState  ·  rolling context (15)  ·  price/time RING BUFFERS   │
│    PolicyState (2)   ·  CockpitState (6)                                   │
│    [FIXES: 8 unbounded collections -> bounded ring buffers]               │
│                                                                           │
│  Sequential per observation -- DIRECT CALLS, not channels, not topics:    │
│    validate causal sequence                                               │
│    adaptive:  D01.step -> D02 build_return_shape -> D04 capturability      │
│               -> rolling-15 -> adaptive decision                          │
│    pricing:   causal quadratic (CURRENT INDEX ONLY)                       │
│               jp from ring buffer                                          │
│               F4 ridge fit (CURRENT INDEX ONLY)                            │
│               [FIXES: O(n^2) full-history recompute -> O(1) per step]     │
│                                                                           │
│               ┌── RK45 ── Phase 3-4 ONLY, if Go integrator not yet ready ─┐│
│               │  bounded semaphore -> narrow STATELESS call               ││
│               │  ~22 float64 in  /  ~33 float64 + status out              ││
│               └──────────────────────────────────────────────────────────┘│
│                                                                           │
│               eigenvalues + expm amplification (gonum)                    │
│               numerical row -> PriceEngine -> EmissionPolicy -> cockpit    │
│    build IO_Vector(OUTPUT) with lineage                                   │
│    [FIXES: pricing outputs have no lineage today]                         │
└─────────────────────────────────┬─────────────────────────────────────────┘
                                  │  per-channel egress channel (bounded)
                                  ▼
                        Bounded fan-in  /  local Pub/Sub
                        [FIXES: unbuffered shared merge that couples channels]
                                  │
      ┌───────────────┬───────────┴────────────┬──────────────────┐
      ▼               ▼                        ▼                  ▼
  persistence     telemetry            future Volume path   future Decision
  sink            (OpenTelemetry)      (subscribes, no       Engine boundary
                                        runtime redesign)    (OUT OF SCOPE)
                                  │
                                  ▼
                          Output IO_Vector
```

### The six refinements, and why

1. **The Python computation service is not in the steady-state path.** The
   target diagram places it inline. Evidence shows only `solve_ivp` lacks a Go
   path (146 lines of 4,643), and it is *stateless*. So it is a bounded,
   optional, temporary bridge in Phases 3–4, not a permanent architectural
   stage. In the target state it is absent entirely.

2. **"Pub/Sub topics *or* typed channels" resolves to typed channels for
   ingress→owner, and Pub/Sub only at the output.** Ingress→owner has exactly
   one consumer per channel, so a topic adds indirection without fan-out. The
   output genuinely has multiple independent consumer families, which is where
   Pub/Sub earns its place.

3. **"Adaptive Go routines" and "pricing Go routines" are sequential calls
   inside one owner goroutine, not separate concurrent services.** Evidence:
   `run_pricing_001.py:118–119` already calls them synchronously; pricing depends
   on adaptive output for the same observation; both mutate per-channel state.
   They can never run concurrently for the same channel. Separating them would
   split per-channel scientific state across two owners — the worst outcome.

4. **"State management" is not a component; it is a property of ownership.** The
   diagram implies a state-management subsystem. The evidence supports the
   opposite: because SADE has no global mutable state and all state is
   instance-attached (J.1), state management reduces to "the owner goroutine
   holds its own state". No subsystem, no locks, no coordination.

5. **The ring-buffer requirement is architectural, not an optimisation.** Eight
   unbounded collections and the O(n²) recompute are the difference between
   feasible and infeasible at scale. They belong in the diagram.

6. **Lineage and ingress timestamping are explicit ingress responsibilities.**
   Neither exists today, and both are prerequisites for the traceability
   requirement (§22) and any latency claim (§33).

## DIAGRAM 4 — PROPOSED IO_VECTOR CHANNEL / PUB-SUB FLOW

```text
IO_Vector(INPUT)  io_vector_id=V1  channel_id=IO_VECTOR_000001  sequence=n
        │
        │  ingress: bounded channel, single shared queue
        ▼
   [ partition by channel_id ]  ── fan-out, key-preserving ──┐
        │                    │                    │          │
        ▼                    ▼                    ▼          ▼
  ch IO_VECTOR_000001  ch IO_VECTOR_000002  ...  ch IO_VECTOR_00000N
   (bounded, cap=k)      (bounded, cap=k)         (bounded, cap=k)
        │                    │                         │
        ▼                    ▼                         ▼
  ┌───────────┐        ┌───────────┐             ┌───────────┐
  │ owner g/r │        │ owner g/r │             │ owner g/r │
  │  state_1  │        │  state_2  │    ...      │  state_N  │
  └─────┬─────┘        └─────┬─────┘             └─────┬─────┘
        │                    │                         │
        │  SEQUENTIAL WITHIN A CHANNEL -- causal order guaranteed
        │  by single-goroutine execution, no locks, no coordination
        │
        │  stage transitions are DIRECT CALLS inside the goroutine:
        │     IO_Vector_Received -> Adaptive_State_Updated
        │                        -> Pricing_State_Updated
        │                        -> Output_Vector_Ready
        │  (these are logical stages, NOT messages, NOT topics)
        │
        ▼                    ▼                         ▼
  egress_1 (bounded)   egress_2 (bounded)   ...   egress_N (bounded)
        │                    │                         │
        └────────────────────┴─────────────────────────┘
                             │  BOUNDED fan-in
                             ▼
                    ┌──────────────────┐
                    │ local Pub/Sub    │   THIS is where topics earn their place:
                    │ broker           │   multiple INDEPENDENT consumer families
                    └────────┬─────────┘   need the same output vector
                             │
        ┌────────────┬───────┴────────┬─────────────────┐
        ▼            ▼                ▼                 ▼
   persistence   telemetry      future Volume     future Decision
   subscriber    subscriber     subscriber        Engine subscriber
                                                  (OUT OF SCOPE)

  IO_Vector(OUTPUT)  io_vector_id=V3  parent=V2  input_io_vector_id=V1
                     channel_id=IO_VECTOR_000001  role=OUTPUT

  ORDERING CONTRACT:
    guaranteed:     total order within a channel_id
    NOT guaranteed: any order across different channel_id values
    rationale:      matches single-goroutine ownership locally AND
                    Event Hubs per-partition ordering on Azure
```

## DIAGRAM 5 — PROPOSED MINIMAL PYTHON RESIDUE

```text
BEFORE (today)                          AFTER PHASE 4                 TARGET
──────────────                          ─────────────                 ──────

┌────────────────────┐                  ┌──────────────────┐      ┌──────────────┐
│ Go: SDX            │                  │ Go: SADE_Go      │      │ Go: SADE_Go  │
│   833 LOC          │                  │   ingestion      │      │   EVERYTHING │
│   ingestion only   │                  │   IO_Vector      │      │              │
└─────────┬──────────┘                  │   partitioning   │      │  incl. Go    │
          │ gRPC, per-vector            │   ordering       │      │  Dormand-    │
          │ protobuf marshal            │   concurrency    │      │  Prince RK45 │
          ▼                             │   state          │      │              │
┌────────────────────┐                  │   D01/D02/D04    │      └──────────────┘
│ Python: SADE       │                  │   derivatives    │
│   4,643 LOC        │                  │   F4 ridge fit   │       Python: NONE
│                    │                  │   eigen + expm   │
│   ALL orchestration│                  │   policy+cockpit │       ~97% Go
│   ALL mathematics  │                  │   output+lineage │
│   ALL state        │                  └────────┬─────────┘
│   ALL serialization│                           │  ~22 float64
│   ALL config       │                           ▼
└────────────────────┘                  ┌──────────────────┐
                                        │ Python           │
  Go 15.2% / Python 84.8%               │  RK45 ONLY       │
                                        │  ~40 LOC         │
                                        │  scipy solve_ivp │
                                        │  STATELESS       │
                                        └────────┬─────────┘
                                                 │  ~33 float64 + status
                                                 ▼
                                          Go processing resumes

                                          Go ~96% / Python ~3%
```

Residue detail at Phase 4:

```text
        Go computes and owns everything except this box:

        ┌──────────────────────────────────────────────────┐
        │  IN:  beta[4], means[3], scales[3],              │
        │       initial_state[3] = [p, p1, p2],            │
        │       rtol, epsilon                    (~22 f64) │
        │                                                  │
        │  scipy.integrate.solve_ivp(                      │
        │      fun    = affine jerk field,                 │
        │      t_span = (0.0, 1.0),                        │
        │      y0     = initial_state,                     │
        │      method = "RK45",                            │
        │      rtol   = 1e-6,                              │
        │      atol   = [rtol*s0,                          │
        │                min(rtol*s1, 0.1*epsilon),        │
        │                rtol*s2],                         │
        │      t_eval = linspace(0, 1, 11))                │
        │                                                  │
        │  OUT: trajectory[11][3], nfev, success, message  │
        │                                        (~35 f64) │
        │                                                  │
        │  STATELESS · PURE · NO per-channel affinity      │
        │  freely poolable, restartable, deletable         │
        └──────────────────────────────────────────────────┘

        Deliberately NOT in the box (all Go, all stdlib-feasible):
          instability rejection  (projection.py:120-122)
          D_local_maximum        (projection.py:123)
          envelope-exit detection(projection.py:124-137)
        -> shrinks projection.py's 146 lines to ~40 at the boundary
```

## DIAGRAM 6 — LOCAL → AZURE EVOLUTION

```text
STAGE 1 -- TODAY                     STAGE 2 -- SADE_Go LOCAL
────────────────                     ────────────────────────

CSV files (5, compiled-in)           SOURCE_000001..N adapters
      │                                    │
      ▼                                    ▼
Go SDX process                       ingress chan (bounded)
      │ gRPC per vector                    │
      ▼                              partition by channel_id
Python SADE process                        │
      │ single thread                 N owner goroutines
      ▼                              (all state goroutine-local)
CSV / JSON files                           │
                                     bounded fan-in -> local Pub/Sub
1 channel                                  │
                                     output sinks
                                     1,000s of channels, ONE process


STAGE 3 -- SADE_Go MULTI-INSTANCE    STAGE 4 -- AZURE
─────────────────────────────────    ────────────────

SOURCE_000001..N adapters            source adapters in Azure compute
      │                                    │
      ▼                                    ▼
local broker w/ partition keys        ╔═══════════════════════════╗
      │                               ║ Azure Event Hubs          ║
      │ (transport abstraction         ║   INGRESS ONLY            ║
      │  boundary established here)    ║   partition key=channel_id║
      ▼                                ║   + durable replay        ║
instance A: channels  1..500          ╚═════════════╤═════════════╝
instance B: channels 501..1000                      │
      │                                 ┌───────────┼───────────┐
      ▼                                 ▼           ▼           ▼
output broker                     consumer A   consumer B   consumer C
                                  partitions   partitions   partitions
                                    0..3         4..7        8..11
                                      │            │            │
                                  ┌───┴────────────┴────────────┴───┐
                                  │ IDENTICAL Go runtime inside each│
                                  │  local channels                 │
                                  │  owner goroutine per channel    │
                                  │  identical scientific code      │
                                  │  identical per-channel state    │
                                  │  ← NOTHING CHANGES HERE →       │
                                  └───┬────────────┬────────────┬───┘
                                      │            │            │
                                      ▼            ▼            ▼
                                  ╔═══════════════════════════════╗
                                  ║ Event Hubs / Service Bus      ║
                                  ║   EGRESS ONLY                 ║
                                  ╚═══════════════╤═══════════════╝
                                                  ▼
                                        Azure storage / downstream

  WHAT CHANGES BETWEEN STAGE 3 AND 4:  ingress edge, egress edge, config, auth
  WHAT DOES NOT CHANGE:                the entire processing graph,
                                       the ownership model,
                                       the ordering guarantee,
                                       every line of scientific code

  EVENT HUBS MUST NOT SIT:  adaptive->pricing · D01->D02->D04
                            owner->its own state · around the RK45 call
                            between internal stages generally
```

---

# Part N — Migration Phases

Refined from the suggested sequence based on evidence. Three substantive
departures from the suggested ordering are marked and justified.

### PHASE 0 — Freeze the validated baseline

- Tag both repositories. **Commit the SDX Go implementation** — it is currently
  entirely untracked (only design docs are committed), which is a provenance
  risk for a scientific baseline.
- Preserve `output/unit_runs/**` as the immutable numerical reference.
- Extend the frozen baseline: capture per-observation intermediate values (`p1`,
  `p2`, `jp`, `means`, `scales`, `standardized`, `physical`, `condition`,
  trajectory, eigenvalues, amplification, and full D01 state at every step) for
  a long deterministic replay. **The current artifacts record only summary
  counters and a flattened observation row — insufficient to validate a Go
  reimplementation function by function.** This is the single most important
  Phase 0 action.
- No code change.

### PHASE 1 — Fix the two scale blockers *in place*, in Python

**DEPARTURE 1 from the suggested sequence.** The suggested Phase 1 begins the Go
foundation. The evidence argues for fixing the O(n²) recomputation and the
unbounded state growth *first, in Python*, because:

- Both are mathematically neutral. Computing only the current index is provably
  identical, since each window fit depends only on its own trailing window and
  `pipeline.py` reads only `active_index`. Bounding history to
  `f4_window + derivative_window + margin` retains every value the mathematics
  reads.
- Both can be validated against the existing frozen baseline *immediately*,
  with no cross-language comparison — the cleanest possible equivalence test.
- Leaving them until after the Go port means the Go implementation would either
  inherit them or diverge from the baseline for two reasons at once
  (language + algorithm), making drift attribution impossible.
- They are the dominant scale bottlenecks (K.1), so fixing them early makes
  every later measurement meaningful.

Scope: replace full-history recomputation with current-index computation; replace
append-forever lists with ring buffers; stop accumulating audits/traces/
emissions. Re-run the baseline and require bit-identical output.

### PHASE 2 — Go runtime foundation

- `IO_Vector` domain type (Part I.3) with encode/decode kept at the edges,
  decoupled from generated protobuf types (K.5).
- Ingress with `SourceAdapter` interface, durable per-channel sequence,
  ingress timestamp, and runtime source/channel registration — removing hard-
  coding items 1, 2, 3, 13 (K.6).
- Partition/ownership registry; owner goroutine per channel; per-channel bounded
  queues with configurable capacity (item 4).
- Per-channel cancellation scope and bounded fan-in, replacing the shared
  `cancel()` and unbuffered merge (items 5, 6).
- Lineage fields populated at ingress.
- Telemetry via OpenTelemetry (already in the module graph).
- Synthetic `IO_Vector` generator (Part O.3).
- Scientific code untouched; the Python pipeline still runs behind the existing
  gRPC boundary. The Go runtime is proven on transport, ordering, partitioning
  and scale with no mathematics in it.

### PHASE 3 — Move non-scientific Python into Go, plus the stdlib-only mathematics

**DEPARTURE 2.** The suggested sequence separates "non-scientific orchestration"
(Phase 2) from "low-risk scientific mathematics" (Phase 3). The evidence
supports merging most of them, because the entire adaptive path is stdlib-only:
D01 (966) + D02 (184) + D04 (239) + emitter (452) = 1,841 lines with zero NumPy
and zero SciPy. There is no library boundary to cross and no linear algebra to
validate. Splitting the adaptive path across two phases would mean shipping a
half-migrated scientific pipeline across a language boundary for no benefit.

Scope: all G1 (2,502 lines) and all G2 (1,650 lines) — orchestration, mapping,
validation, serialization, configuration, D01, D02, D04, the emitter, the
Price Engine, `EmissionPolicy`, and the cockpit. Function-level equivalence at
declared tolerance, then module-level, then full-pipeline replay.

At the end of Phase 3, the Python residue is `pricing_pipeline/derivatives.py`,
`dynamics.py`, `numerical.py`, `projection.py` — 491 lines.

### PHASE 4 — Move the linear algebra into Go with rigorous equivalence validation

Scope: G3 (345 lines) — `causal_quadratic` via `mat.SVD`; `fit_f4` via
`mat.Dense.Solve` and `mat.Cond(·,2)` with explicit population standard
deviation; `eigvals` via `mat.Eigen`; `expm` via `mat.Dense.Exp`. Plus the
non-SciPy parts of `projection.py` (instability, `D_local_maximum`, envelope
exit), shrinking the Python bridge to ~40 lines.

This is the highest-risk phase. Validation must assert not only float agreement
but **agreement of the derived discrete labels** — confidence tier, stability
state, colour — because those are what the system actually outputs, and they are
step functions of continuous quantities near calibrated thresholds.

Declare gonum in `go.mod` here. No network fetch is required.

### PHASE 5 — Close the Python boundary

Write the Go adaptive Dormand–Prince RK45 with per-component absolute tolerance,
dense output at prescribed evaluation points, and step accept/reject accounting.

**DEPARTURE 3.** Consider pulling this earlier, possibly into Phase 4 or even
Phase 2. Rationale from evidence:

- The test problem is the easiest class of IVP: 3-dimensional, affine,
  constant-coefficient, non-stiff, one unit interval, and it never failed in the
  validated run (55/55).
- An **independent analytic oracle exists** — `expm(A·t)` — giving a third
  reference beyond SciPy and Go. Very few migrations have this.
- If RK45 remains behind a cross-process Python boundary at scale, CPython's
  global interpreter lock forces either serialisation or multiple interpreter
  processes — the "thousands of Python workers" outcome the objective
  explicitly rejects (K.1, bottleneck 2).
- Writing it early means **zero wrapper services are ever needed** (H.4),
  avoiding a process boundary that would otherwise be introduced and then
  removed.

The counter-argument is real: it is the only genuinely new numerical
infrastructure in the programme, and doing it early adds risk to an earlier
phase. This is a sequencing judgement for human decision, and it is flagged as
such.

### PHASE 6 — Local multi-thousand-channel scale validation

- Synthetic generator drives 1, 10, 100, 1,000, then 10,000 channels.
- Per-channel causal order verified under concurrency.
- Backpressure and failure isolation verified per channel.
- Bounded memory verified over sustained multi-hour operation.
- Latency budget populated per component (K.2) — first real numbers.
- Bottlenecks identified from measurement rather than inspection.

### PHASE 7 — Azure integration

- Transport abstraction at ingress/egress only.
- Event Hubs at ingress with `channel_id` as partition key; egress to Event Hubs
  or Service Bus.
- Multi-instance channel ownership by partition assignment.
- Replay validated as a scientific capability, not just an operational one.
- Local↔Azure transport equivalence tests (Part O.2 J).
- Transport security configured, removing hard-coding item 15.

Explicitly **not** in any phase: the Go Decision Engine, order rules, paper or
execution control, the Volume Pipeline, and the semantic layer.

---

# Part O — Scientific Freeze, Test Strategy, Scale Test Data

## O.1 Scientific freeze and required equivalence evidence

Governing principle: any Go refactor of scientific mathematics must preserve the
current validated mathematical behaviour. **No mathematical simplification may
be introduced to make the Go implementation easier.**

Three concrete traps identified where a "cleaner" Go implementation would
silently change the science:

1. `structural_quality` uses `x ** (1.0/3.0)`. Substituting `math.Cbrt` — which
   is *more* accurate — produces different values. Must use
   `math.Pow(x, 1.0/3.0)`.
2. `fit_f4` uses `np.std` with `ddof=0` (population). Substituting
   `stat.StdDev` (sample, Bessel-corrected) changes every scale by
   `sqrt(30/29)` ≈ 1.0174 and therefore changes every coefficient. Must
   implement population standard deviation explicitly.
3. `solve_cover` solves an affine constant-coefficient ODE whose analytic
   solution the codebase already computes via `expm`. Replacing RK45 with the
   closed form would be a simplification and is forbidden — but the closed form
   is invaluable as a *validation oracle*.

A fourth, subtler trap: `CapturabilityModelV0_2.validate_return_shape` requires
**exact float equality** between recomputed and stored displacement values
(`capturability_model.py:36–39`). The Go implementation must compute them in the
same operation order or this check fails.

Required equivalence evidence per migration candidate:

| Evidence | Requirement |
|---|---|
| Fixed input vectors | Deterministic, versioned, committed. Must include boundary cases: `dt = 0`; `dt > 5.0` (gap counter); zero and negative velocity; `|p1| ≤ epsilon`; `max_abs_displacement = 0` (ZERO_GEOMETRY); `scales ≤ 0` (F4 skip); `rank ≠ 3` (lstsq failure); condition number straddling each policy threshold; `max_real_eigenvalue` straddling zero; RK45 failure; domain exit. |
| Source Python output | Captured from the frozen baseline **before** any change, at full intermediate granularity — not just summary counters. Phase 0 gap. |
| Go output | Same inputs, same fixed order. |
| Tolerance | Declared per operation, not global. Proposal: exact for comparison/classification results; `1e-15` relative for `sqrt`-only paths; `1e-12` relative for `exp`/`pow` paths; `1e-10` relative for dense linear algebra; `1e-8` relative for RK45 trajectories. **And exact agreement for every derived discrete label** — this is the requirement that actually matters. |
| State equivalence | Full per-channel state compared **after every observation** over a long replay, not just at the end. D01's recursive state, the rolling context, `PolicyState` and `CockpitState` must all match. Single-step equivalence is insufficient for a recursive model. |
| Deterministic replay | Same input sequence must produce identical output on repeat runs in both languages. Requires fixed map-iteration order wherever iteration affects results — note `update_parameters` iterates `params.items()` (`adaptation.py:16`), which is currently a single-key dict but would become order-sensitive if extended. |
| Edge cases | Non-finite inputs; empty and short histories; the INITIALIZING→ACTIONABLE transition at exactly 15; the `active_index = index − 1` offset at stream start; first and last observation; cancellation mid-stream. |

## O.2 Test strategy for future migration (design only, not implemented)

**A. FUNCTION EQUIVALENCE TESTS.** Per migrated function, fixed input vectors,
Python baseline vs Go output at declared tolerance. Table-driven. Must cover
every guard branch, not just the happy path — e.g. `causal_quadratic`'s
`rank != 3` path, `fit_f4`'s `scales <= 0` skip, `EmissionPolicy`'s
`not rk_success` branch. Highest-value targets: `fit_f4` (all seven output
arrays independently), `causal_quadratic` (`p1`, `p2`, `failures`, and derived
`jp`), `eigvals`, `expm` amplification, every D01 scalar function, every policy
and cockpit classification.

**B. MODULE EQUIVALENCE TESTS.** Whole-module behaviour: `D01V02Model.step` over
a long sequence comparing all 26 DMO fields, all 8 FMO samples, and the full
`RuntimeState` after **each** step; `build_return_shape` over many DMO/FMO
pairs; `CapturabilityModelV0_2.evaluate` including `reason_codes` ordering;
`EmissionPolicy.emit` and `PriceCockpitInterpreter.observe` including state
transitions and `reason_codes` order (recall the `dict.fromkeys` ordering
dependency).

**C. PIPELINE EQUIVALENCE TESTS.** Full adaptive and pricing pipelines over the
frozen baseline input, asserting every per-observation output field and every
summary counter. Must reproduce the recorded run exactly: 15
WARMUP_DERIVATIVE, 30 WARMUP_F4, 55 emissions, 55/55 RK45 success, 18 domain
exits, 26 LOW / 29 MEDIUM confidence, 33 AMBER / 14 GREEN / 8 RED, 331
adaptation events, 170 feedback events.

**D. IO_VECTOR CAUSAL ORDER TESTS.** Per channel, assert strictly monotonic
sequence at the owner under concurrent load; assert output order matches input
order per channel; assert cross-channel interleaving does **not** violate
per-channel order; assert no observation is dropped, duplicated or reordered.
Include adversarial interleaving and deliberately skewed arrival rates.

**E. MULTI-CHANNEL CONCURRENCY TESTS.** Identical input on N channels must
produce identical per-channel output regardless of N — proving no cross-channel
state leakage. Run under `-race`. Assert no goroutine leaks after channel
retirement (compare `runtime.NumGoroutine()` before and after).

**F. BACKPRESSURE TESTS.** Slow one channel's consumer; assert its queue reaches
capacity and reports BACKPRESSURED; assert other channels continue to
completion (the generalisation of `TestAAPLBackpressureDoesNotStopMSFT` to the
fan-in layer, which is where current isolation breaks); assert memory stays
bounded; assert **no observation is dropped**, since dropping breaks causal
recursion and would be rejected by `assert_causal_sequence` anyway.

**G. FAILURE ISOLATION TESTS.** Fail one source, one channel's mathematics, and
the Python bridge independently; assert unaffected channels continue; assert the
failed channel reports a terminal state with a diagnostic; assert recovery or
clean retirement. This directly targets the current shared-`cancel()` coupling.

**H. THOUSANDS-CHANNEL GO SCALE TESTS.** 1 / 10 / 100 / 1,000 / 10,000 channels
via the synthetic generator. Measure goroutine count, heap size and allocation
rate per channel, GC pause distribution, per-channel and aggregate throughput,
queue depth distributions, and scheduler latency. Assert linear-or-better memory
scaling and no throughput collapse.

**I. END-TO-END NEAR-REAL-TIME TESTS.** Per-component latency distributions
(K.2) populated from real measurement. Requires `ingest_timestamp` to exist
first. Report p50/p95/p99 per component and end to end. **Explicitly account for
the structural one-observation pricing offset**, which is not a performance
property.

**J. LOCAL → AZURE TRANSPORT EQUIVALENCE TESTS.** Identical input through local
channels and through Azure transport must produce identical scientific output.
Assert per-partition ordering is preserved by Event Hubs partition keys; assert
replay from a durable offset reproduces the identical scientific trajectory
(this is the test that proves transport independence); assert
at-least-once delivery semantics do not corrupt recursive state — which requires
either idempotency by `io_vector_id` or exactly-once handling at the owner. **This
last point is a genuine design requirement that Event Hubs adoption forces, and
it does not exist in the current code at all.**

## O.3 Scale test data — synthetic IO_Vector generator (recommendation, not implemented)

Future scale testing must not require thousands of real financial instruments.
Recommend a neutral, deterministic synthetic `IO_Vector` generator.

Identifiers: `IO_VECTOR_000001` … `IO_VECTOR_N`; sources `SOURCE_000001`,
`SOURCE_000002`, …

Required controls:

| Control | Purpose |
|---|---|
| Channel count | scale sweep 1 → 10,000 |
| Vector frequency (per channel) | arrival-rate sweep; independently settable per channel |
| Payload size | serialisation-cost sensitivity |
| Burst behaviour | backpressure and buffer sizing |
| Deterministic sequence | reproducible runs from a fixed seed; identical output across repeat runs and across implementations |
| Injected delay | slow-channel isolation testing |
| Injected failure | failure-isolation testing (malformed payload, sequence gap, source stall, source error) |

Additional properties the evidence says the generator needs:

- **Numerically well-behaved payload series.** The pricing mathematics requires
  non-degenerate windows: `fit_f4` skips any window where a scale is ≤ 0
  (`dynamics.py:96`), so constant series produce no fits at all. A pure
  synthetic constant would silently test nothing. A deterministic non-degenerate
  series is required — the existing equivalence-test fixture
  (`100 + 0.2i + 1.5·sin(i/5)`, `test_pricing_migration_equivalence.py:101`) is a
  reasonable neutral, non-financial precedent.
- **Configurable irregular timestamp spacing.** The real baseline saw gaps of
  60–300 s (`summary.json`), and the system deliberately performs no cadence
  normalisation. The generator must be able to reproduce irregular spacing, and
  must be able to produce a strictly increasing sequence, since
  `emitter.py:195–196` raises on non-increasing source time.
- **Independent per-channel state.** Each synthetic channel must produce an
  independent series so that cross-channel state leakage is detectable — if all
  channels received identical input, leakage would be invisible.
- **No financial semantics whatsoever.** Generic named payload fields, no
  instrument identity, no market concepts.

Not implemented in this task.

---

# Part P — Risk Register

| ID | Risk | Severity | Likelihood | Evidence | Mitigation |
|---|---|---|---|---|---|
| R1 | **Scientific drift during Python→Go migration** | HIGH | HIGH | 1,995 lines of mathematics to reimplement; recursive state amplifies small differences | Phase 0 full-granularity baseline capture; per-function tolerance; per-step state comparison over long replays |
| R2 | **Floating-point differences in transcendentals** | MEDIUM | CERTAIN | `math.exp` in strength/uncertainty/reversal; `math.pow` in curvature/decay/structural quality; CPython libm vs Go pure-Go | Tolerance-based, not bitwise, equivalence; bound accumulation over long replays; never substitute a "better" function (`Cbrt` for `Pow(x,1/3)`) |
| R3 | **Linear algebra differences** | HIGH | HIGH | `lstsq`(SVD) vs QR; `solve`(LU) pivoting; `cond` norm; `eigvals` ordering; `expm` Padé tables | Mirror algorithm choices (`mat.SVD` for `lstsq`, `mat.Cond(·,2)`); validate all seven `fit_f4` outputs independently; test near policy thresholds |
| R4 | **Population vs sample standard deviation mistranslation** | **CRITICAL** | MEDIUM | `np.std` defaults `ddof=0`; `stat.StdDev` applies Bessel correction — a silent 1.74% scale error | Explicit population std; dedicated equivalence test on `scales` alone; code-review checklist item |
| R5 | **RK45 equivalence** | MEDIUM | MEDIUM | No Go implementation exists; per-component `atol`, adaptive stepping, dense output must all match | Three-way validation: SciPy vs Go vs analytic `expm(At)`; exploit that the problem is affine and non-stiff |
| R6 | **Discrete label flips from continuous drift** | **CRITICAL** | HIGH | `cond`/`eigenvalue`/`amplification` feed hard-coded median/q95 thresholds (`policy.py:196–201`); `eigenvalue ≤ 0` sets stability; observed 0 HIGH / 26 LOW / 29 MEDIUM | Assert **label** equivalence, not just float equivalence; test inputs deliberately straddling every threshold; treat any label flip as a migration failure |
| R7 | **State ownership errors during migration** | HIGH | MEDIUM | 30 state objects (J.1), several implicitly keyed by "the one entity" | One owner goroutine per channel; no shared mutable scientific state; `-race` in CI; explicit per-channel state comparison tests |
| R8 | **Per-channel ordering violations** | HIGH | LOW | Single-goroutine ownership makes ordering structural | Test suite D; never round-robin; never a worker pool over stateful work |
| R9 | **Cross-channel blocking** | HIGH | **CERTAIN today** | `router.go:183` unbuffered shared fan-in serialises all channels through one send loop | Per-channel egress; bounded fan-in; test suite F at the fan-in layer, not just `RoutePartitions` |
| R10 | **Cross-channel failure coupling** | HIGH | **CERTAIN today** | `router.go:242` shared `cancel()`; `TestCancellationTerminatesAllBlockedPartitions` asserts it | Per-channel cancellation scope; test suite G |
| R11 | **Backpressure with a causally recursive model** | HIGH | MEDIUM | D01 is recursive; `assert_causal_sequence` and strict index checks reject gaps, so dropping an observation *fails* rather than degrades | Block, never drop; explicit documented overflow policy; per-source admission control decision needs human input |
| R12 | **Unbounded channels / memory growth** | **CRITICAL** | **CERTAIN today** | 8 unbounded collections; 331+170 audit entries per 100 observations | Ring buffers; emit telemetry instead of accumulating; sustained soak test |
| R13 | **Goroutine leakage** | MEDIUM | MEDIUM | 2N+2 goroutines per request today; owner-per-channel adds lifecycle complexity | Explicit retirement on idle; `NumGoroutine` assertions before/after; `context` propagation |
| R14 | **Canonical hash divergence breaks identity** | HIGH | HIGH | `observation_id`/`emission_id` are SHA-256 over `json.dumps(sort_keys, separators)`; Go's `encoding/json` formats floats differently from CPython `repr` | **Human decision required**: either reproduce CPython float repr exactly, or accept new identity values and re-baseline. Do not discover this late. |
| R15 | **`physical_row + 2` identity coupling** | MEDIUM | CERTAIN | `pipeline.py:67` → `emitter.py:227`; embedded in `observation_id` | Human decision: preserve the legacy offset for identity continuity, or re-baseline identities |
| R16 | **O(n²) recomputation** | **CRITICAL** | **CERTAIN today** | `derivatives.py:81`, `dynamics.py:89` refit all history; `pipeline.py` reads one index | Phase 1 fix in Python first, validated bit-identical against the baseline |
| R17 | **Python bottleneck at scale (GIL)** | HIGH | HIGH if RK45 retained | ~55% of observations call `solve_ivp`; CPython serialises on one interpreter | Write Go RK45 early (Phase 5 → consider Phase 2–4); bounded semaphore if a bridge is used |
| R18 | **Serialization overhead in the hot path** | MEDIUM | CERTAIN today | 4 × (`json.dumps` + SHA-256) over large nested dicts per observation; multiple `deepcopy` | Compute identity once; avoid deep copies; benchmark before and after |
| R19 | **gRPC overhead for an in-process handoff** | MEDIUM | CERTAIN today | Every vector marshalled/unmarshalled between two processes | Consolidate into one Go process — removes the hop entirely |
| R20 | **Duplicate processing / at-least-once delivery** | HIGH | MEDIUM on Azure | Event Hubs is at-least-once; D01 is recursive, so a duplicate observation silently corrupts the trajectory | Idempotency by `io_vector_id` at the owner, or exactly-once handling. **Absent from current code; must be designed before Phase 7.** |
| R21 | **Replay behaviour** | MEDIUM | MEDIUM | No checkpointing today; `to_snapshot`/`from_snapshot` exist but `from_snapshot` restores only 5 of ~20 state fields (`snapshot.py:34–40`) | Complete state snapshot/restore; validate replay-from-checkpoint reproduces the trajectory |
| R22 | **Failure recovery absent** | HIGH | CERTAIN today | No retry, no resume, no checkpoint anywhere; a failed run restarts from row 0 | Per-channel restart with state restore; durable ingress offsets |
| R23 | **Azure partitioning mismatch** | MEDIUM | MEDIUM | Local ownership is registry-based; Event Hubs assigns by key hash — channel→instance mapping is not controllable | Use `channel_id` as partition key; design for arbitrary partition assignment; never assume co-location of specific channels |
| R24 | **Premature microservice decomposition** | HIGH | MEDIUM | The wrapper-service discussion; the temptation to keep SDX as a deployed service | Apply the §7 criteria explicitly; H.4 finds no wrapper permanently justified and H.5 finds no current network boundary justified |
| R25 | **Premature abstraction** | MEDIUM | MEDIUM | `IO_Vector` could accumulate speculative fields; `enums.py` already carries 4 unused enums | Keep `IO_Vector` to evidence-backed fields only (I.3); add on demonstrated need |
| R26 | **Hidden financial hard-coding surviving into the generic runtime** | HIGH | HIGH | `entity="AAPL"` as a **production** config default; compile-time symbol slice; `ToUpper` on identifiers; provider-specific filename pattern; OHLCV-typed payload | Hard-coding audit (K.6) as a Phase 2 exit gate; grep gate in CI for instrument symbols in non-test code |
| R27 | **Fabricated inputs influencing science** | HIGH | CERTAIN today | `data_valid="true"` → `source_quality=1.0` → perturbation classification and uncertainty; `session_type="UNKNOWN"`; `receive_time = event_time` | Represent unknown explicitly rather than asserting a value; **requires scientific review**, since removing the assumption changes output |
| R28 | **Untracked SDX implementation** | MEDIUM | CERTAIN today | `git status`: `cmd/`, `internal/`, `gen/`, `proto/`, `go.mod`, `go.sum` all untracked; only design docs committed | Commit in Phase 0 before any migration work |
| R29 | **Latency claims without measurement** | MEDIUM | HIGH | Zero benchmarks; one instrumented stage whose data is discarded; no ingress timestamp | Mark all latency NOT YET MEASURED until Phase 6; add `ingest_timestamp` in Phase 2 |
| R30 | **Dead-but-broken code paths** | LOW | CERTAIN today | `time_term=True` would `KeyError`; `derivative_state` unused; `DevelopmentObservationStream` unused; 4 unused enums; `_closed`/`start_stream`/`cancel_stream` unused | Decide explicitly per item: port, or drop with a recorded rationale |

---

# Part Q — Required Direct Answers

**1. What percentage of current production-relevant SADE executable code is
Python vs Go by LOC?**
Handwritten production code: Go 833 lines (15.2%), Python 4,643 lines (84.8%).
Including generated bindings: Go 1,955 (28.4%), Python 4,928 (71.6%). SADE
itself contains **zero Go**; all Go is in SDX.

**2. What percentage of runtime RESPONSIBILITY is currently Python vs Go?**
Higher Python share than LOC suggests. Go owns 4 of ~28 runtime
responsibilities (L.4): source ingestion, vector construction, ingress
partitioning/concurrency, transport — plus partial ownership of ordering,
lifecycle, error control and configuration. Python owns everything else:
all mathematics, all scientific state, all runtime state, orchestration,
serialization, output, and the remainder of configuration. Roughly **85–90%
Python by responsibility.**

**3. Which Python modules can move to Go immediately?**
All 34 G1 modules (2,502 lines): `sade/__init__.py`, `__main__.py`, both
`adaptive_pipeline` modules, `adaptive_emitter/normalizer.py`,
`configuration/scientific_baseline.py`, both `input` modules (eliminated
entirely), `d01/v02/{config,state,observations,outputs,snapshot,trace}.py`,
`d02/v02/models.py`, all 4 `d04/models` modules, `d04/__init__.py`,
`pricing_pipeline/pipeline.py` orchestration, `price_engine/{contracts,engine}.py`,
both `unit_run` harnesses, and all namespace `__init__.py` files. None contains
floating-point scientific computation.

**4. Which scientific Python functions can move to Go with low risk?**
All 20 G2 modules (1,650 lines): every D01 mathematical function (reference,
kinematics, innovation, volume, coherence, strength, persistence, uncertainty,
reversal, perturbation, half-life, adaptation, forward, health, and `model.step`
sequencing), `d02/v02/builder.py`, `d04/envelope/capturability_model.py`,
`adaptive_emitter/emitter.py`, `price_engine/policy.py`,
`price_engine/cockpit.py`. All are stdlib-only. Risk is confined to 1–2 ulp
differences in `exp`/`pow`.

**5. Which require rigorous equivalence testing?**
The 3 G3 modules (345 lines): `derivatives.py` (`np.linalg.lstsq`),
`dynamics.py` (`np.linalg.solve`, `np.linalg.cond`, population `np.std`),
`numerical.py` (`np.linalg.eigvals`, `scipy.linalg.expm`). These feed discrete
classification thresholds, so continuous drift can flip categorical output.

**6. Which should remain Python initially?**
Exactly one: `sade/pricing_pipeline/projection.py` (146 lines), and within it
only the `scipy.integrate.solve_ivp` RK45 call — roughly 40 lines once the
non-SciPy diagnostics move to Go.

**7. Is RK45 the primary reason Python remains?**
**Yes, and it is the only reason.** Every other Python dependency either has a
gonum equivalent already extracted in the local module cache
(`lstsq`→`mat.SVD`, `solve`→`mat.Dense.Solve`, `cond`→`mat.Cond`,
`eigvals`→`mat.Eigen`, `expm`→`mat.Dense.Exp`) or is stdlib-only.

**8. Are there other strong reasons Python remains?**
No strong ones. Three weak ones, all engineering rather than scientific:
(a) `scipy.linalg.expm` is battle-tested and `mat.Dense.Exp` needs validation —
but the algorithm family is the same; (b) canonical SHA-256 identity values
depend on CPython's float repr, so preserving byte-identical
`observation_id`/`emission_id` is fiddly in Go — an identity-continuity concern,
not a mathematical one; (c) pydantic gives declarative validation that Go
requires explicit code for — ergonomics, not capability.

**9. What is the smallest plausible Python runtime boundary after migration?**
One stateless numerical call: approximately **22 float64 in** (β[4], means[3],
scales[3], initial state[3], rtol, epsilon) and **~33 float64 plus status out**
(11×3 trajectory, nfev, success, message). No state, no per-channel affinity,
no scientific ownership.

**10. How much of the current Adaptive Pipeline can become Go?**
**All of it — 100%.** D01 (966) + D02 (184) + D04 (239) + emitter (452) +
adaptive_pipeline (431) = 2,272 lines, with zero NumPy and zero SciPy. The only
non-stdlib import is pydantic, used for validation.

**11. How much of the current Pricing Pipeline can become Go?**
**1,574 of 1,720 lines (91.5%).** `pricing_pipeline/` (898) + `price_engine/`
(822) = 1,720; only `projection.py` (146) lacks a Go path. All 822 lines of
`price_engine/` are stdlib-only.

**12. Can D01 be implemented in Go without new mathematical design?**
**Yes.** Every equation is closed-form and fully specified. No solver, no
fitting, no iteration to convergence. Markovian scalar recursion, stdlib only.

**13. Can D02 be implemented in Go without new mathematical design?**
**Yes.** Mostly validation plus four arithmetic expressions.

**14. Can D04 be implemented in Go without new mathematical design?**
**Yes.** Four arithmetic expressions plus eligibility logic. pydantic validation
becomes explicit Go validation. Preserve the provenance invariant and the exact
float-equality check in `validate_return_shape`.

**15. Can Price derivative mathematics move to Go?**
**Yes**, with equivalence validation. `causal_quadratic` needs a 15×3 least
squares — use `mat.SVD` to mirror `np.linalg.lstsq`'s SVD-based rank criterion.
`derivative_state` is stdlib-only and currently unused.

**16. Can F4 move to Go?**
**Yes**, and it is the **highest-risk** migration. `mat.Dense.Solve` for the 4×4
ridge system, `mat.Cond(·, 2)` for the condition number, and an **explicit
population standard deviation** — `stat.StdDev` would be wrong. Normal-equation
formation squares the condition number, and `cond` feeds discrete confidence
thresholds.

**17. Can PriceEngine move to Go?**
**Yes, trivially.** `engine.py` is a 39-line coherence gate with no mathematics.
`policy.py` and `cockpit.py` are stdlib-only. All of `price_engine/` (822 lines)
moves with no library dependency. Watch the `dict.fromkeys` ordered-dedupe for
`reason_codes`.

**18. Can cockpit logic move to Go?**
**Yes.** 248 lines, stdlib only, two guarded divisions, state is already an
explicit external transition.

**19. What current SDX code should survive unchanged?**
Conceptually: `router.go`'s per-partition-queue + producer-goroutine +
blocking-send backpressure pattern; `main.go`'s signal handling and graceful-stop
sequence; the `PartitionState`/`SourceState` status model; and the discipline of
forwarding source timestamps verbatim with no assigned semantics
(`sdx.proto:63`) — this last is a genuinely valuable design decision. The test
suite's concurrency assertions should also survive as the basis for the scale
tests.

**20. What current SDX code should be expanded/refactored?**
`SDReader` → a `SourceAdapter` interface (remove CSV-only and the fixed header);
`main.go` → runtime source/channel registration (remove the compile-time symbol
slice and the provider filename pattern); `router.Route` → per-channel egress
with bounded fan-in (remove the unbuffered shared merge) and per-channel
cancellation (remove the shared `cancel()`); `server.validateRequest` → remove
`ToUpper` on identifiers; `MarketVector` → `IO_Vector` with generic payload,
lineage, role, ingress timestamp and durable sequence; `DefaultCapacity` →
configuration; reader status → per-stream rather than per-source.

**21. Should SDX remain separate from SADE_Go?**
**Not as a separately deployed network service.** It should become Go packages
(`ingress/source`, `ingress/router`) inside one consolidated `SADE_Go` runtime —
option C. The gRPC hop exists only because SADE is Python; once the runtime is
Go it is pure cost (protobuf marshal/unmarshal per vector for an in-process
handoff). Applying the §7 criteria, no independent scaling, fault isolation,
language, security or deployment-lifecycle justification survives. Reintroduce a
network boundary later only for a specific external source that genuinely
requires process isolation.

**22. What should the canonical IO_Vector contain?**
Minimum from Part I.3: identity (`io_vector_id`, `channel_id`, `sequence`);
`role` (INPUT|OUTPUT); provenance (`source_id`, `source_timestamp` verbatim,
`ingest_timestamp`); payload (`payload`, `payload_schema`, `availability`);
`source_quality`; lineage (`parent_io_vector_id`, `input_io_vector_id`,
`context_io_vector_ids`); status (`status`, `error`). Explicitly excluded: any
scientific state, configuration, transport/topic metadata, decision or order
fields, and any instrument or market concept.

**23. How should IO_Vector partition ownership work?**
`channel_id` is the sole partition key. A registry maps `channel_id` to exactly
one owner goroutine, created on first sight and retired on idle. Fan-out is
strictly key-preserving — never round-robin. The owner holds all state for its
channel, goroutine-locally, with no locks. The same key becomes the Event Hubs
partition key on Azure, so the ownership model is unchanged by transport.

**24. How should causal order be guaranteed?**
By construction, through single-goroutine ownership per channel — ordering
becomes a consequence of execution rather than something enforced by
synchronisation. Backed by ingress-assigned monotonic sequence, retention of the
existing explicit sequence checks (`observations.py:45`, `pipeline.py:128`) as
assertions, and a stated contract: total order **within** a `channel_id`, no
order guarantee **across** channels.

**25. How should thousands of channels be represented?**
As N owner goroutines with bounded input and output channels, plus a registry —
**not** as OS processes, microservices, or Python workers. At N = 1,000:
~1,000–2,100 goroutines (~8–16 MB of stacks) and 10–20 MB of bounded
per-channel state. SDX already runs 2N+2 goroutines for N partitions, so the
pattern is established rather than speculative.

**26. Are Go channels sufficient locally?**
**Yes**, for everything in-process. Bounded channels already provide the exact
backpressure semantics the current SDX reader relies on, and blocking sends are
the correct overflow policy here because the mathematics is causally recursive
and cannot tolerate dropped observations.

**27. Where is Pub/Sub useful?**
Two places only: **ingress fan-out** by `channel_id`, and **output
distribution** to multiple independent consumer families (persistence,
telemetry, future Volume path, future Decision Engine). The output case is the
one that genuinely needs topics, and it is what makes the architecture
extensible without redesign.

**28. Where is fan-out required?**
Ingress → channel owners (key-preserving), and output → multiple consumer
families.

**29. Where is fan-in required?**
Per-channel egress → persistence, and per-channel egress → telemetry
aggregation. Both must be **bounded** and must not recreate the current
unbuffered single-channel merge that couples all channels.

**30. What backpressure strategy is recommended?**
Bounded queue **per channel**, blocking send, never drop (dropping breaks causal
recursion and would fail the sequence assertions anyway), per-channel
cancellation scope, per-source admission control, bounded fan-in with
per-channel egress, configurable capacity sized from measurement, and observable
queue depth / time-in-backpressure via OpenTelemetry. Extends the existing
`BACKPRESSURED` partition state rather than replacing it.

**31. Are wrapper microservices needed initially?**
At most **one**, and only narrowly: a stateless RK45 bridge, if the Go
integrator is not written first. `Adaptive_Pipeline_Wrapper` is **NOT REQUIRED** —
the entire adaptive path is stdlib-only, so a stateful Python wrapper would add
a process boundary and a state-affinity problem for no benefit. A whole-pricing-
pipeline wrapper is also **NOT REQUIRED**. If Phase 5 is pulled earlier, **zero
wrapper services are ever needed.**

**32. Are wrapper microservices needed permanently?**
**No.** Neither is permanently justified. The narrow RK45 bridge, if built, is a
`TEMPORARY MIGRATION BRIDGE` that disappears once the Go integrator passes
equivalence.

**33. What would the narrowest Python service API look like?**
A single stateless call: in — β[4], means[3], scales[3], initial state[3],
rtol, epsilon (~22 float64); out — trajectory[11][3], nfev, success flag,
message (~33 float64 + status). No session, no state, no channel affinity, no
scientific ownership. Freely poolable and deletable.

**34. What are the current likely throughput bottlenecks?**
In order: (1) the **O(n²) full-history recomputation** in `causal_quadratic` and
`fit_f4` — dominates everything and is mathematically free to fix; (2) the
**Python boundary / GIL** if RK45 stays cross-process at ~55% of observations;
(3) **unbounded memory growth** in 8 collections — a hard failure, not a
slowdown; (4) four JSON+SHA-256 passes and multiple `deepcopy` per observation;
(5) **per-vector gRPC serialisation** across the SDX→SADE hop; (6)
**cross-channel head-of-line blocking** at the unbuffered fan-in; (7) pydantic
model construction per observation. Notably, the actual numerical work is tiny —
**every bottleneck is structural, not mathematical.**

**35. What should be measured before claiming near-real-time?**
First, add an `ingest_timestamp` — end-to-end latency is currently unmeasurable
in principle because `receive_time` is fabricated as `= event_time`. Then per
component (K.2): p50/p95/p99 latency, per-channel and aggregate throughput,
queue depth and time-in-queue, goroutine count, heap size and allocation rate
per channel, GC pause distribution, RK45 calls/s and per-call latency, and
end-to-end ingest→emit distribution. And explicitly account for the structural
one-observation pricing offset, which is architectural rather than a performance
property.

**36. What local architecture maps most cleanly to Azure?**
One Go process with: a transport-abstracted ingress, `channel_id` as the sole
partition key, one owner goroutine per channel holding all its state, local
typed channels for every internal hop, and a transport-abstracted egress. Then
only the two edges change on Azure: each consumer instance runs the identical Go
runtime and owns the channels mapped to its assigned partitions. Every line of
scientific code is unchanged.

**37. Where should Azure Event Hubs eventually sit?**
Source ingress (with `channel_id` as partition key), cross-instance transport,
output egress for durable downstream consumers, burst buffering, scale-out, and
**replay** — the last being especially valuable because D01 is recursive, so
deterministic replay from a durable ordered log is both an operational and a
scientific capability the system currently lacks entirely.

**38. Where should Event Hubs NOT sit?**
Between Adaptive and Pricing (sequential stages on the same channel sharing
per-channel state — a transport there would split scientific state across
consumers); between D01, D02 and D04 (three calls in one scientific step, with a
bitwise-coupled value between D02 and D04); between an owner and its own state;
around the RK45 call (a stateless 22-float numerical routine does not belong
behind a durable log); and between internal processing stages generally.

**39. What code hard-coding must be removed before scale testing?**
Ten blockers from K.6: the compile-time entity slice (`main.go:23`); the
provider filename pattern (`main.go:31`); the fixed CSV header
(`reader.go:16`); `DefaultCapacity = 10` (`router.go:15`); the unbuffered shared
fan-in (`router.go:183`); the shared `cancel()` (`router.go:242`);
`entity="AAPL"` as a **production** config default (`pipeline.py:191`,
`__main__.py:49`); the fabricated `data_valid="true"`/`session_type="UNKNOWN"`
(`pipeline.py:105–106`); the fabricated `receive_time = event_time`
(`normalizer.py:68`); `strings.ToUpper` on channel identifiers
(`server.go:167`, `router.go:96`). Plus the pervasive single-channel-per-process
assumption. Items 10, 11 and 12 additionally require **scientific** review, not
just engineering cleanup, because `data_valid="true"` becomes
`source_quality = 1.0` and influences perturbation classification and
uncertainty, and `physical_row + 2` is baked into `observation_id`.

**40. What is the recommended migration order?**
Phase 0 freeze **with full-granularity baseline capture** (the current artifacts
are insufficient); Phase 1 fix the O(n²) recompute and unbounded state **in
Python first**, validated bit-identical; Phase 2 Go runtime foundation
(`IO_Vector`, ingress, ownership, per-channel backpressure, lineage, telemetry,
synthetic generator) with no mathematics in it; Phase 3 all G1 + G2 including
the entire adaptive path and the entire Price Engine; Phase 4 the G3 linear
algebra with rigorous equivalence; Phase 5 the Go RK45 — **consider pulling this
earlier**; Phase 6 local thousands-channel scale validation and first real
latency numbers; Phase 7 Azure.

**41. What is the highest-risk Go migration?**
`sade/pricing_pipeline/dynamics.py::fit_f4`. Three compounding risks: normal-
equation formation squares the condition number; the `ddof` and matrix-norm
mismatches are silent and severe (`stat.StdDev` would introduce a 1.74% scale
error; `mat.Cond` requires an explicit norm); and its `condition_number` output
feeds hard-coded median/q95 thresholds, so continuous drift produces
**discrete** label flips. Not RK45 — RK45 is the highest-*effort* migration, and
it has an independent analytic oracle that `fit_f4` does not.

**42. What is the lowest-risk Go migration?**
`sade/pricing_pipeline/price_engine/` — all 822 lines are stdlib-only,
containing only comparisons and subtractions with no transcendentals, no linear
algebra and no library dependency. `PolicyState` and `CockpitState` are already
pure external state transitions, which is exactly the shape a Go owner goroutine
wants. Equally low-risk on the adaptive side: `coherence.py`, `innovation.py`
and `perturbation.py`, whose only numerical operation is exactly-rounded `sqrt`.

**43. What is the recommended SADE_Go V0.1 scope?**
See Part R.

**44. What should explicitly NOT be in SADE_Go V0.1?**
See Part R.

**45. Based on executable evidence, is the target of a predominantly Go runtime
realistic?**
**Yes — and more so than the 90/10 target implies.** The evidence: 96.9% of
current SADE production Python has an identified Go path (G1 + G2 + G3 = 4,497
of 4,643 lines); the entire adaptive path, the entire Price Engine and all
orchestration are stdlib-only or pure plumbing; every linear-algebra operation
has a gonum equivalent already extracted in the local module cache; only 146
lines depend on a Python capability with no Go equivalent, and that dependency
is a *stateless* call on an affine, non-stiff, 3-dimensional ODE that has an
analytic reference solution; SADE has no global mutable state to untangle; and
SDX already demonstrates and tests concurrent multi-channel ingestion with
per-channel ordering. The realistic end state is **~96–97% Go**. The binding
constraints are not feasibility but (a) the cost of scientific equivalence
validation, (b) the two language-independent structural blockers, and (c) the
one-off effort of writing a Go RK45 integrator.

---

# Part R — Recommended SADE_Go V0.1 Boundary

## GO V0.1 SHOULD OWN

Runtime and infrastructure:

- Source ingestion via a `SourceAdapter` interface, with local CSV as the first
  adapter (generalising `SDReader`).
- Canonical `IO_Vector` domain type with role, generic payload, lineage,
  provenance and per-vector status — decoupled from generated protobuf types.
- Ingress: normalisation, durable per-channel sequence assignment,
  `ingest_timestamp` stamping, `io_vector_id` and lineage population.
- Partition/ownership registry keyed by opaque `channel_id`; owner creation on
  first sight, retirement on idle.
- One owner goroutine per `IO_Vector` channel, holding all per-channel state
  goroutine-locally with no locks.
- Key-preserving fan-out; bounded per-channel input and egress channels;
  bounded fan-in; local Pub/Sub at the output only.
- Per-channel bounded backpressure with configurable capacity, blocking sends,
  and no dropping.
- Per-channel cancellation, failure isolation, lifecycle and recovery.
- All runtime state: ordering counters, queue depth, channel status, health.
- Bounded scientific state: ring buffers replacing every unbounded collection.
- Serialization at boundaries only; output `IO_Vector` construction.
- Configuration for all operational values (ports, capacities, sources,
  channels, timeouts).
- Observability via OpenTelemetry: per-stage latency, throughput, queue depth,
  goroutine count, heap, per-channel status.
- Deterministic synthetic `IO_Vector` generator for scale testing.

Scientific mathematics (after phased equivalence validation):

- All of D01 — 23 modules, 966 lines, stdlib-only.
- All of D02 — 4 modules, 184 lines, stdlib-only.
- All of D04 — 7 modules, 239 lines, stdlib-only mathematics plus explicit
  validation replacing pydantic, preserving the provenance invariant.
- The adaptive emitter — rolling-15 context, adaptive properties, decision
  predicate, state transitions, audit emission as telemetry.
- Causal quadratic derivatives via `mat.SVD` (equivalence-validated).
- F4 ridge fitting via `mat.Dense.Solve`, `mat.Cond(·,2)` and explicit
  population standard deviation (equivalence-validated; highest risk).
- Companion-matrix eigenvalues via `mat.Eigen` and perturbation amplification
  via `mat.Dense.Exp` (equivalence-validated).
- RK45 post-processing: instability rejection, `D_local_maximum`, envelope-exit
  detection — all stdlib-feasible, moved out of the Python bridge.
- The entire Price Engine: coherence gate, `EmissionPolicy`,
  `PriceCockpitInterpreter`, `PolicyState`, `CockpitState` — 822 lines,
  stdlib-only.

## PYTHON V0.1 SHOULD RETAIN

- **`scipy.integrate.solve_ivp(..., method="RK45")` — and nothing else.**

Scope: approximately 40 lines behind a stateless call taking ~22 float64 and
returning ~33 float64 plus status. No state, no channel affinity, no scientific
ownership beyond the integration step itself.

Sole justification: no adaptive Runge–Kutta initial-value solver exists anywhere
in the local Go dependency set (verified against the extracted gonum tree and
the full module graph), and faithful reproduction requires the adaptive step
controller, the per-component absolute tolerance vector, dense output at
prescribed evaluation points, and `nfev`/`message` reporting.

Explicitly **not** retained in Python: D01, D02, D04, the adaptive emitter, any
orchestration, any state ownership, any serialization, any configuration,
`scipy.linalg.expm` (gonum has `mat.Dense.Exp`), or any NumPy linear algebra
(gonum covers all five operations used).

## MIGRATION BRIDGES REQUIRED

**One, conditionally:**

- **Narrow stateless RK45 bridge** — `TEMPORARY MIGRATION BRIDGE`. Required
  only for Phases 3–4, and only if the Go Dormand–Prince integrator is not
  written first. Bounded concurrency via semaphore. Deleted at Phase 5.

**None permanently.**

Explicitly **NOT REQUIRED** at any point:

- `Adaptive_Pipeline_Wrapper` — the entire adaptive path is stdlib-only; a
  stateful Python wrapper would add a process boundary and a per-channel
  state-affinity problem in exchange for nothing.
- `Pricing_Pipeline_Wrapper` (whole-pipeline scope) — would split per-channel
  scientific state across a process boundary and become a permanent affinity
  constraint.
- Any separately deployed network microservice, including SDX as a service. No
  §7 criterion is satisfied by the current evidence.

If Phase 5 is pulled earlier, **zero bridges are ever built** — worth serious
consideration, since it avoids introducing a boundary that must later be removed.

## DEFERRED

- Volume Pipeline.
- Go Decision Engine.
- Paper / execution control.
- Semantic layer.
- System-level feedback expansion.
- Azure production deployment.

# Recommended First Implementation Sequence

Recommended, **not implemented**. No code was written for this task.

1. **Commit the SDX Go implementation.** It is currently entirely untracked
   (only design docs are committed). A scientific baseline cannot rest on
   untracked source.
2. **Extend the frozen baseline to full numerical granularity.** Capture, for a
   long deterministic replay, every per-observation intermediate: `p1`, `p2`,
   `jp`, `means`, `scales`, `standardized`, `physical`, `condition`, the full
   RK45 trajectory, eigenvalues, amplification, and the complete D01
   `RuntimeState` after each step. **The existing artifacts record only summary
   counters and a flattened observation row, which is insufficient to validate a
   Go reimplementation function by function.** This is the highest-value first
   action and it changes no code.
3. **Human decisions required before any migration begins.** Four items:
   (a) must `observation_id`/`emission_id` remain byte-identical, which
   determines whether Go must reproduce CPython's float repr (risk R14);
   (b) should the legacy `physical_row + 2` convention be preserved for identity
   continuity (R15); (c) is the fabricated `data_valid="true"` →
   `source_quality = 1.0` acceptable, given it influences perturbation
   classification and uncertainty (R27); (d) how should per-source admission
   control behave when one source feeds many channels (R11).
4. **Fix the O(n²) recomputation in Python.** Compute only the current index in
   `causal_quadratic` and `fit_f4`. Provably identical, since each window fit
   depends only on its trailing window and only `active_index` is read. Require
   bit-identical output against the baseline.
5. **Bound all state in Python.** Ring buffers for price and time history sized
   to `f4_window + derivative_window + margin`; stop accumulating audits,
   traces and emissions. Require bit-identical output.
6. **Re-run and re-freeze the baseline.** Now with a validated, bounded, O(1)-
   per-observation reference.
7. **Define the `IO_Vector` type in Go**, with encode/decode confined to the
   edges and no generated protobuf type in internal signatures.
8. **Build the Go ingress and ownership layer.** `SourceAdapter` interface,
   runtime channel registration, durable sequence, `ingest_timestamp`, lineage,
   per-channel bounded queues with configurable capacity, key-preserving
   fan-out, per-channel cancellation, bounded fan-in. No mathematics.
9. **Build the deterministic synthetic `IO_Vector` generator.** Non-degenerate
   series (constant series produce no F4 fits at all), configurable irregular
   timestamp spacing, independent per-channel state, injectable delay and
   failure, no financial semantics.
10. **Validate the Go runtime at scale with no mathematics in it.** 1 → 10,000
    channels: per-channel ordering, failure isolation, backpressure isolation at
    the fan-in layer, bounded memory, goroutine accounting. Proves the runtime
    before any scientific risk is taken.
11. **Migrate G1 to Go** — orchestration, mapping, validation, serialization,
    configuration, contracts. No numerical risk.
12. **Migrate G2 to Go** — D01, D02, D04, the adaptive emitter, `EmissionPolicy`,
    the cockpit. Function-level, then module-level, then full-replay equivalence
    with per-step state comparison. Transcribe every inline scientific constant
    exactly; use `math.Pow(x, 1.0/3.0)`, never `math.Cbrt`.
13. **Write the Go RK45 (Dormand–Prince) early.** Validate three ways: against
    SciPy, against the analytic `expm(A·t)` solution, and against the frozen
    baseline. Doing this now means no bridge is ever built.
14. **Migrate G3 to Go** — `mat.SVD`, `mat.Dense.Solve`, `mat.Cond(·,2)` with
    explicit population standard deviation, `mat.Eigen`, `mat.Dense.Exp`. Declare
    gonum in `go.mod` (already cached and hash-pinned; no network fetch).
    Assert **discrete label** equivalence, not merely float agreement.
15. **Remove all invalid runtime hard-coding** as an explicit exit gate, with a
    CI check for instrument symbols in non-test code.
16. **Populate the latency budget** from real measurement. Only then discuss
    near-real-time targets, accounting explicitly for the structural
    one-observation pricing offset.
17. **Abstract transport at ingress and egress only**, then evaluate Azure Event
    Hubs at those two edges — and design `io_vector_id` idempotency at the owner
    before adopting at-least-once delivery.

---

# Integrity Check

Targeted verification of the no-change constraint.

```text
SADE CODE MODIFIED:
NO

SDX CODE MODIFIED:
NO

TESTS MODIFIED:
NO

SCIENTIFIC RUNS:
NO

NEW CODE:
NO

NEW DOCUMENTS:
EXACTLY ONE
```

The one new document is:

```text
docs/investigations/
SADE_GO_REFACTORABILITY_AND_SCALED_RUNTIME_INVESTIGATION_2026-08-27.md
```

No untracked files were deleted. No generated outputs, historical evidence, run
artifacts or test artifacts were removed. No repository cleanup, reset or
workspace state change was performed. The only filesystem additions are the
`docs/investigations/` directory and this document.
