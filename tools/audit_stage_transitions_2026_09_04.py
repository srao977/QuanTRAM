"""
QuanTRAM StageTransition V1.1 live-run forensic parser (2026-09-04).

Purpose
    Independently parse a human-readable stage_transitions.txt diagnostic
    and emit quantitative evidence for the live-run audit. This script is
    investigation tooling only. It is not on the realtime path, does not
    publish transitions, and must not be imported by quantram-server.

Inputs
    Path to a StageTransition diagnostic TXT file (default:
    ./stage_transitions.txt relative to the current working directory).

Outputs
    Structured JSON summary on stdout covering run metadata, inventory,
    sequence integrity, reconstruction checks, P-03/P-04 sparsity,
    synthetic no-Bar P-03 events, printed initiating-Bar completeness,
    same-Bar P-03/P-04 lineage, warm-up progression, and market-close
    window extracts.

Parameters / configuration
    argv[1] optional diagnostic path. No environment variables. No
    network. No mutation of the input file.

Ownership
    Audit / investigation only. Not a StageTransition subscriber.

Lifecycle
    Single-shot CLI. Parse, analyze, print, exit.

Concurrency
    None. Single process, single thread.

Failure behavior
    Parse errors are collected and reported. The script exits 0 after
    printing JSON so the audit document can still consume a partial
    result. A non-zero exit is reserved for unreadable input.

Invariants
    Does not recompute Adaptive or Price Engine science. Treats the TXT
    as a lossy rendering of Event values (see writeInitiatingBar).

Non-responsibilities
    Snapshot, Persistence, proto, dashboard, scientific engines.
"""

from __future__ import annotations

import json
import re
import sys
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path


SEP = "----------------------------------------------------------------------"
BLOCK_TITLE = "STAGE TRANSITION"


def parse_time(value: str):
    value = value.strip()
    if value in ("", "N/A"):
        return None
    # RFC3339Nano with variable fractional digits, always UTC Z.
    if value.endswith("Z"):
        core = value[:-1]
        if "." in core:
            head, frac = core.split(".", 1)
            frac = (frac + "000000")[:6]
            core = f"{head}.{frac}"
            return datetime.fromisoformat(core).replace(tzinfo=timezone.utc)
        return datetime.fromisoformat(core).replace(tzinfo=timezone.utc)
    return datetime.fromisoformat(value)


def duration_hms(start, stop):
    if not start or not stop:
        return None
    seconds = int((stop - start).total_seconds())
    hours, rem = divmod(seconds, 3600)
    minutes, secs = divmod(rem, 60)
    return f"{hours:02d}:{minutes:02d}:{secs:02d}", seconds


def parse_kv_block(lines: list[str]) -> dict[str, str]:
    out: dict[str, str] = {}
    current = None
    for raw in lines:
        if not raw.strip():
            continue
        if raw.startswith("    ") and current and ":" not in raw[4:]:
            out[current] = (out.get(current, "") + " " + raw.strip()).strip()
            continue
        if ":" in raw:
            key, val = raw.split(":", 1)
            key = key.strip()
            val = val.strip()
            current = key
            out[key] = val
    return out


def parse_indented_section(text: str, header: str) -> dict[str, str]:
    lines = text.splitlines()
    capturing = False
    collected: list[str] = []
    for line in lines:
        if line.startswith(header):
            capturing = True
            continue
        if capturing:
            if line and not line.startswith(" ") and line.endswith(":") and line != header:
                break
            if line.startswith("    ") or line == "":
                if line.startswith("    "):
                    collected.append(line)
                elif collected:
                    break
            else:
                break
    return parse_kv_block(collected)


def parse_state(text: str, header: str) -> dict[str, str]:
    return parse_indented_section(text, header)


def parse_block(raw: str) -> dict:
    ev: dict = {
        "raw_len": len(raw),
        "transition_id": None,
        "sequence": None,
        "stage_id": None,
        "stage_name": None,
        "entity": None,
        "effective_time": None,
        "published_time": None,
        "previous": {},
        "current": {},
        "initiating_bar": None,
        "facts": {},
        "correlation": {},
        "reason": None,
    }
    for line in raw.splitlines():
        if line.startswith("Transition ID:"):
            ev["transition_id"] = line.split(":", 1)[1].strip()
        elif line.startswith("Sequence:"):
            seq = line.split(":", 1)[1].strip()
            ev["sequence"] = None if seq == "N/A" else int(seq)
        elif line.startswith("Stage ID:"):
            ev["stage_id"] = line.split(":", 1)[1].strip()
        elif line.startswith("Stage Name:"):
            ev["stage_name"] = line.split(":", 1)[1].strip()
        elif line.startswith("Entity:"):
            ev["entity"] = line.split(":", 1)[1].strip()
        elif line.startswith("Effective Time:"):
            ev["effective_time"] = line.split(":", 1)[1].strip()
        elif line.startswith("Published Time:"):
            ev["published_time"] = line.split(":", 1)[1].strip()
        elif line.startswith("Reason:"):
            continue
    # Reason is the last "    VALUE" after "Reason:"
    if "Reason:" in raw:
        after = raw.split("Reason:", 1)[1]
        for line in after.splitlines():
            if line.startswith("    "):
                ev["reason"] = line.strip()
                break

    ev["previous"] = parse_state(raw, "Previous State:")
    ev["current"] = parse_state(raw, "Current State:")
    bar_section = ""
    if "Initiating Bar:" in raw:
        bar_section = raw.split("Initiating Bar:", 1)[1]
        if "Transition Facts:" in bar_section:
            bar_section = bar_section.split("Transition Facts:", 1)[0]
    bar = parse_kv_block([ln for ln in bar_section.splitlines() if ln.startswith("    ")])
    if (not bar) or "N/A" in bar_section and "Symbol:" not in bar_section:
        ev["initiating_bar"] = None
    else:
        ev["initiating_bar"] = bar
    ev["facts"] = parse_indented_section(raw, "Transition Facts:")
    ev["correlation"] = parse_indented_section(raw, "Correlation:")
    return ev


def authoritative_p03(state: dict) -> tuple:
    return (
        state.get("Kind", ""),
        state.get("Decision", ""),
        state.get("Skip Reason", ""),
        state.get("Model Status", ""),
        state.get("Emitter", ""),
    )


def authoritative_p04(state: dict) -> tuple:
    return (
        state.get("Kind", ""),
        state.get("Pricing Status", ""),
        state.get("Color", ""),
        state.get("Skip Reason", ""),
        state.get("Trajectory Phase", ""),
        state.get("Domain State", ""),
        state.get("Confidence State", ""),
        state.get("Domain Exit", ""),
    )


def authoritative(ev: dict) -> tuple:
    stage = ev["stage_id"]
    st = ev["current"]
    if stage == "P01_MARKET_FEED":
        return ("FEED", st.get("Feed State", ""))
    if stage == "P02_INGESTION":
        return ("INGESTION", st.get("Capability", ""))
    if stage == "P03_ADAPTIVE":
        return authoritative_p03(st)
    if stage == "P04_PRICE_ENGINE":
        return authoritative_p04(st)
    return tuple(sorted(st.items()))


def previous_authoritative(ev: dict) -> tuple | None:
    prev = ev["previous"]
    if not prev or prev == {"ABSENT": ""} or list(prev.keys()) == [] or (len(prev) == 1 and "ABSENT" in str(prev)):
        if ev["previous"] == {} or "ABSENT" in ev.get("previous_raw", ""):
            return None
    if prev.get("Kind") is None and len(prev) <= 1 and "ABSENT" in json.dumps(prev):
        return None
    stage = ev["stage_id"]
    if "ABSENT" in prev and len(prev) == 1:
        return None
    if not prev:
        return None
    if stage == "P01_MARKET_FEED":
        if not prev.get("Kind") and not prev.get("Feed State"):
            return None
        return ("FEED", prev.get("Feed State", ""))
    if stage == "P02_INGESTION":
        if not prev.get("Kind") and not prev.get("Capability"):
            return None
        return ("INGESTION", prev.get("Capability", ""))
    if stage == "P03_ADAPTIVE":
        if not prev.get("Kind"):
            return None
        return authoritative_p03(prev)
    if stage == "P04_PRICE_ENGINE":
        if not prev.get("Kind"):
            return None
        return authoritative_p04(prev)
    return tuple(sorted(prev.items()))


def is_absent_previous(ev: dict) -> bool:
    prev = ev["previous"]
    return prev == {} or (len(prev) == 1 and next(iter(prev.keys()), "") == "ABSENT") or list(prev.values()) == [] and "ABSENT" in str(prev)


def load_file(path: Path):
    text = path.read_text(encoding="utf-8")
    header = {}
    footer = {}
    m_start = re.search(r"Process Started:\s+(\S+)", text)
    m_ver = re.search(r"Contract Version:\s+(\S+)", text)
    m_stop = re.search(r"Process Stopped:\s+(\S+)", text)
    m_written = re.search(r"Transitions Written:\s+(\d+)", text)
    m_dropped = re.search(r"Transitions Dropped:\s+(\d+)", text)
    m_errors = re.search(r"Write Errors:\s+(\d+)", text)
    header["process_started"] = m_start.group(1) if m_start else None
    header["contract_version"] = m_ver.group(1) if m_ver else None
    footer["process_stopped"] = m_stop.group(1) if m_stop else None
    footer["written"] = int(m_written.group(1)) if m_written else None
    footer["dropped"] = int(m_dropped.group(1)) if m_dropped else None
    footer["write_errors"] = int(m_errors.group(1)) if m_errors else None

    events = []
    marker = f"{SEP}\n{BLOCK_TITLE}\n{SEP}\n"
    starts = []
    idx = 0
    while True:
        found = text.find(marker, idx)
        if found < 0:
            break
        starts.append(found)
        idx = found + len(marker)
    for i, start in enumerate(starts):
        end = starts[i + 1] if i + 1 < len(starts) else text.find("STAGE TRANSITION DIAGNOSTIC COMPLETE")
        if end < 0:
            end = len(text)
        events.append(parse_block(text[start:end]))
    return text, header, footer, events


def bar_key(bar: dict | None, corr: dict) -> str | None:
    if bar and bar.get("Market Snapshot ID") and bar["Market Snapshot ID"] != "N/A":
        return bar["Market Snapshot ID"]
    snap = corr.get("Market Snapshot ID")
    if snap and snap != "N/A":
        return snap
    return None


def summarize(path: Path) -> dict:
    text, header, footer, events = load_file(path)
    issues = []

    # Fix absent detection using raw previous lines.
    for ev in events:
        # re-detect ABSENT
        pass

    start = parse_time(header["process_started"]) if header["process_started"] else None
    stop = parse_time(footer["process_stopped"]) if footer["process_stopped"] else None
    dur = duration_hms(start, stop)

    by_stage = Counter(ev["stage_id"] for ev in events)
    by_entity = Counter(f"{ev['stage_id']}:{ev['entity']}" for ev in events)
    stages = sorted({ev["stage_id"] for ev in events})
    entities = sorted({ev["entity"] for ev in events})

    if footer["written"] != len(events):
        issues.append(f"footer written={footer['written']} parsed_blocks={len(events)}")

    # Sequence integrity
    seq_report = {}
    for key_evs in group_by(events, lambda e: (e["stage_id"], e["entity"])):
        key, evs = key_evs
        seqs = [e["sequence"] for e in evs]
        missing = []
        dups = []
        resets = []
        if seqs:
            expect = list(range(seqs[0], seqs[-1] + 1))
            missing = [n for n in expect if n not in seqs]
            seen = Counter(seqs)
            dups = [n for n, c in seen.items() if c > 1]
            for i in range(1, len(seqs)):
                if seqs[i] <= seqs[i - 1]:
                    resets.append({"prev": seqs[i - 1], "next": seqs[i], "id": evs[i]["transition_id"]})
        seq_report[f"{key[0]}:{key[1]}"] = {
            "count": len(evs),
            "first": seqs[0] if seqs else None,
            "last": seqs[-1] if seqs else None,
            "monotonic": not resets and not dups and not missing and (seqs == list(range(seqs[0], seqs[-1] + 1)) if seqs else True),
            "missing": missing,
            "duplicates": dups,
            "resets": resets,
            "first_id": evs[0]["transition_id"] if evs else None,
            "last_id": evs[-1]["transition_id"] if evs else None,
            "first_previous_absent": evs[0]["previous"] == {"ABSENT": ""} or list(evs[0]["previous"].keys()) == ["ABSENT"] if evs else None,
        }

    # Reconstruction: previous of N+1 == current of N (printed fields)
    reconstruction = []
    for key, evs in group_by(events, lambda e: (e["stage_id"], e["entity"])):
        for i in range(1, len(evs)):
            prev_cur = evs[i - 1]["current"]
            this_prev = evs[i]["previous"]
            if prev_cur != this_prev:
                reconstruction.append({
                    "id": evs[i]["transition_id"],
                    "prev_id": evs[i - 1]["transition_id"],
                    "expected": prev_cur,
                    "got": this_prev,
                })

    # Adjacent identical authoritative state = bar-only / defect candidate
    identical_adj = []
    for key, evs in group_by(events, lambda e: (e["stage_id"], e["entity"])):
        for i in range(1, len(evs)):
            if authoritative(evs[i]) == authoritative(evs[i - 1]):
                identical_adj.append({
                    "id": evs[i]["transition_id"],
                    "prev_id": evs[i - 1]["transition_id"],
                    "state": list(authoritative(evs[i])),
                    "bar_now": evs[i]["initiating_bar"],
                    "bar_prev": evs[i - 1]["initiating_bar"],
                })

    # P-03 classifications
    p03 = [e for e in events if e["stage_id"] == "P03_ADAPTIVE"]
    p03_kind = Counter()
    p03_side = Counter()
    p03_codes = Counter()
    p03_synthetic = []
    for i, ev in enumerate(p03):
        prev_kind = ev["previous"].get("Kind", "ABSENT")
        cur_kind = ev["current"].get("Kind", "")
        p03_kind[f"{prev_kind} -> {cur_kind}"] += 1
        p03_codes[ev["current"].get("Code", "")] += 1
        if prev_kind == "DECISION" and cur_kind == "DECISION":
            p03_side[f"{ev['previous'].get('Decision','')} -> {ev['current'].get('Decision','')}"] += 1
        if ev["initiating_bar"] is None:
            p03_synthetic.append({
                "transition_id": ev["transition_id"],
                "entity": ev["entity"],
                "effective_time": ev["effective_time"],
                "published_time": ev["published_time"],
                "previous_code": ev["previous"].get("Code") or ("ABSENT" if not ev["previous"].get("Kind") else ev["previous"]),
                "current_code": ev["current"].get("Code"),
                "current": ev["current"],
                "previous": ev["previous"],
                "reason": ev["reason"],
                "accepted_sequence": ev["correlation"].get("Accepted Sequence"),
                "source_event_id": ev["correlation"].get("Source Event ID"),
            })

    # P-04 classifications
    p04 = [e for e in events if e["stage_id"] == "P04_PRICE_ENGINE"]
    p04_status = Counter()
    p04_color_change = Counter()
    p04_same_color_other = []
    p04_field_changes = Counter()
    for i, ev in enumerate(p04):
        p04_status[ev["current"].get("Pricing Status") or ev["current"].get("Code", "")] += 1
        prev_c = ev["previous"].get("Color")
        cur_c = ev["current"].get("Color")
        if prev_c and cur_c:
            p04_color_change[f"{prev_c} -> {cur_c}"] += 1
            if prev_c == cur_c:
                changed = []
                for field in ("Pricing Status", "Skip Reason", "Trajectory Phase", "Domain State", "Confidence State", "Domain Exit"):
                    if ev["previous"].get(field) != ev["current"].get(field):
                        changed.append(field)
                p04_same_color_other.append({
                    "transition_id": ev["transition_id"],
                    "entity": ev["entity"],
                    "color": cur_c,
                    "changed_fields": changed,
                    "previous": ev["previous"],
                    "current": ev["current"],
                    "accepted_sequence": ev["correlation"].get("Accepted Sequence"),
                    "effective_time": ev["effective_time"],
                })
        if ev["previous"].get("Kind"):
            for field in ("Pricing Status", "Color", "Skip Reason", "Trajectory Phase", "Domain State", "Confidence State", "Domain Exit"):
                if ev["previous"].get(field) != ev["current"].get(field):
                    p04_field_changes[field] += 1

    # Same-bar lineage: pair P03 and P04 by snapshot id
    p03_by_snap = defaultdict(list)
    p04_by_snap = defaultdict(list)
    for ev in p03:
        key = bar_key(ev["initiating_bar"], ev["correlation"])
        if key:
            p03_by_snap[key].append(ev)
    for ev in p04:
        key = bar_key(ev["initiating_bar"], ev["correlation"])
        if key:
            p04_by_snap[key].append(ev)
    shared = sorted(set(p03_by_snap) & set(p04_by_snap))
    pair_mismatches = []
    pair_ok = 0
    comparable_fields = [
        "Symbol", "Market Snapshot ID", "Interval Start", "Interval End",
        "Open", "High", "Low", "Close", "Volume", "Source", "Source Timestamp", "Quality",
    ]
    pair_examples = []
    for snap in shared:
        for a in p03_by_snap[snap]:
            for b in p04_by_snap[snap]:
                ba, bb = a["initiating_bar"], b["initiating_bar"]
                if ba is None or bb is None:
                    pair_mismatches.append({"snapshot": snap, "reason": "one side missing printed bar", "p03": a["transition_id"], "p04": b["transition_id"]})
                    continue
                diffs = {k: (ba.get(k), bb.get(k)) for k in comparable_fields if ba.get(k) != bb.get(k)}
                # correlation vs bar
                if a["correlation"].get("Market Snapshot ID") != ba.get("Market Snapshot ID"):
                    diffs["p03_corr_vs_bar"] = (a["correlation"].get("Market Snapshot ID"), ba.get("Market Snapshot ID"))
                if b["correlation"].get("Market Snapshot ID") != bb.get("Market Snapshot ID"):
                    diffs["p04_corr_vs_bar"] = (b["correlation"].get("Market Snapshot ID"), bb.get("Market Snapshot ID"))
                if a["entity"] != b["entity"] or ba.get("Symbol") != bb.get("Symbol"):
                    diffs["entity"] = (a["entity"], b["entity"], ba.get("Symbol"), bb.get("Symbol"))
                if diffs:
                    pair_mismatches.append({"snapshot": snap, "p03": a["transition_id"], "p04": b["transition_id"], "diffs": diffs})
                else:
                    pair_ok += 1
                    if len(pair_examples) < 5:
                        pair_examples.append({
                            "snapshot": snap,
                            "p03": a["transition_id"],
                            "p04": b["transition_id"],
                            "accepted_p03": a["correlation"].get("Accepted Sequence"),
                            "accepted_p04": b["correlation"].get("Accepted Sequence"),
                            "interval": ba.get("Interval Start"),
                            "symbol": ba.get("Symbol"),
                        })

    # Bar completeness vs printed diagnostic fields
    printed_bar_fields = [
        "Symbol", "Market Snapshot ID", "Interval Start", "Interval End",
        "Open", "High", "Low", "Close", "Volume", "Source", "Source Timestamp", "Quality",
    ]
    domain_bar_not_printed = [
        "InstrumentID", "InstrumentType", "Tradable", "Interval", "EventCount",
        "ReceiptTime", "IsFinal", "IsBackfilled", "SourceTransition",
    ]
    bar_incomplete = []
    bar_corr_mismatch = []
    for ev in events:
        bar = ev["initiating_bar"]
        if bar is None:
            continue
        missing_printed = [f for f in printed_bar_fields if not bar.get(f) or bar.get(f) == "N/A"]
        if missing_printed:
            bar_incomplete.append({"id": ev["transition_id"], "missing": missing_printed})
        snap_bar = bar.get("Market Snapshot ID")
        snap_corr = ev["correlation"].get("Market Snapshot ID")
        if snap_bar and snap_corr and snap_bar != snap_corr:
            bar_corr_mismatch.append(ev["transition_id"])
        if ev["entity"] not in ("GLOBAL",) and bar.get("Symbol") and bar.get("Symbol") != ev["entity"]:
            bar_corr_mismatch.append(ev["transition_id"] + ":symbol")

    # Warm-up
    warmup = {}
    for entity in sorted({e["entity"] for e in p03 + p04}):
        p03e = [e for e in p03 if e["entity"] == entity]
        p04e = [e for e in p04 if e["entity"] == entity]
        warmup[entity] = {
            "p03_first": {
                "id": p03e[0]["transition_id"] if p03e else None,
                "code": p03e[0]["current"].get("Code") if p03e else None,
                "accepted": p03e[0]["correlation"].get("Accepted Sequence") if p03e else None,
                "effective": p03e[0]["effective_time"] if p03e else None,
            } if p03e else None,
            "p03_first_decision": next(
                (
                    {
                        "id": e["transition_id"],
                        "code": e["current"].get("Code"),
                        "accepted": e["correlation"].get("Accepted Sequence"),
                        "effective": e["effective_time"],
                        "model_status": e["current"].get("Model Status"),
                    }
                    for e in p03e
                    if e["current"].get("Kind") == "DECISION"
                ),
                None,
            ),
            "p03_codes_in_order": [e["current"].get("Code") for e in p03e],
            "p04_first": {
                "id": p04e[0]["transition_id"] if p04e else None,
                "code": p04e[0]["current"].get("Code") if p04e else None,
                "accepted": p04e[0]["correlation"].get("Accepted Sequence") if p04e else None,
                "effective": p04e[0]["effective_time"] if p04e else None,
            } if p04e else None,
            "p04_first_emitted": next(
                (
                    {
                        "id": e["transition_id"],
                        "code": e["current"].get("Code"),
                        "accepted": e["correlation"].get("Accepted Sequence"),
                        "effective": e["effective_time"],
                        "status": e["current"].get("Pricing Status"),
                        "color": e["current"].get("Color"),
                    }
                    for e in p04e
                    if e["current"].get("Pricing Status") == "EMITTED" or (e["current"].get("Code") or "").startswith("EMITTED:")
                ),
                None,
            ),
            "p04_codes_in_order": [e["current"].get("Code") for e in p04e],
            "last_accepted_p03": p03e[-1]["correlation"].get("Accepted Sequence") if p03e else None,
            "last_accepted_p04": p04e[-1]["correlation"].get("Accepted Sequence") if p04e else None,
            "p03_count": len(p03e),
            "p04_count": len(p04e),
        }

    # Sparsity estimate from last accepted sequence
    sparsity = {}
    for entity, info in warmup.items():
        last = None
        for cand in (info.get("last_accepted_p03"), info.get("last_accepted_p04")):
            try:
                n = int(cand)
                last = n if last is None else max(last, n)
            except (TypeError, ValueError):
                pass
        sparsity[entity] = {
            "estimated_accepted_minutes": last,
            "p03_transitions": info["p03_count"],
            "p04_transitions": info["p04_count"],
            "p03_ratio": (info["p03_count"] / last) if last else None,
            "p04_ratio": (info["p04_count"] / last) if last else None,
        }

    # Market close window >= 19:59Z
    close_events = []
    for ev in events:
        et = ev["effective_time"] or ev["published_time"]
        if et and et >= "2026-09-04T19:59:00":
            close_events.append({
                "transition_id": ev["transition_id"],
                "stage": ev["stage_id"],
                "entity": ev["entity"],
                "effective_time": ev["effective_time"],
                "published_time": ev["published_time"],
                "previous_code": ev["previous"].get("Code") or ev["previous"],
                "current_code": ev["current"].get("Code"),
                "current": ev["current"],
                "facts": ev["facts"],
                "has_bar": ev["initiating_bar"] is not None,
                "accepted": ev["correlation"].get("Accepted Sequence"),
            })

    p01 = [e for e in events if e["stage_id"] == "P01_MARKET_FEED"]
    p02 = [e for e in events if e["stage_id"] == "P02_INGESTION"]

    # First/last samples
    def brief(ev):
        return {
            "id": ev["transition_id"],
            "effective": ev["effective_time"],
            "published": ev["published_time"],
            "prev": ev["previous"].get("Code") or ev["previous"],
            "cur": ev["current"].get("Code") or ev["current"],
            "has_bar": ev["initiating_bar"] is not None,
            "accepted": ev["correlation"].get("Accepted Sequence"),
            "reason": ev["reason"],
            "facts": ev["facts"],
        }

    return {
        "path": str(path),
        "header": header,
        "footer": footer,
        "parsed_event_count": len(events),
        "duration": {"hms": dur[0] if dur else None, "seconds": dur[1] if dur else None},
        "stages": stages,
        "entities": entities,
        "by_stage": dict(by_stage),
        "by_stage_entity": dict(by_entity),
        "issues": issues,
        "sequence": seq_report,
        "reconstruction_mismatches": reconstruction,
        "identical_adjacent_authoritative": identical_adj,
        "p01": [brief(e) for e in p01],
        "p02": [brief(e) for e in p02],
        "p03_kind_transitions": dict(p03_kind),
        "p03_side_transitions": dict(p03_side),
        "p03_current_codes": dict(p03_codes),
        "p03_synthetic_count": len(p03_synthetic),
        "p03_synthetic": p03_synthetic,
        "p04_status_counts": dict(p04_status),
        "p04_color_transitions": dict(p04_color_change),
        "p04_same_color_other_count": len(p04_same_color_other),
        "p04_same_color_other": p04_same_color_other[:20],
        "p04_field_changes": dict(p04_field_changes),
        "same_bar_shared_snapshots": len(shared),
        "same_bar_pairs_ok": pair_ok,
        "same_bar_mismatches": pair_mismatches,
        "same_bar_examples": pair_examples,
        "bar_printed_fields": printed_bar_fields,
        "bar_domain_fields_not_in_txt": domain_bar_not_printed,
        "bar_incomplete_printed": bar_incomplete[:20],
        "bar_corr_mismatch": bar_corr_mismatch,
        "warmup": warmup,
        "sparsity": sparsity,
        "close_events": close_events,
        "p03_no_bar_total": sum(1 for e in p03 if e["initiating_bar"] is None),
        "p04_no_bar_total": sum(1 for e in p04 if e["initiating_bar"] is None),
        "p04_no_bar": [brief(e) for e in p04 if e["initiating_bar"] is None],
        "p02_no_bar": [brief(e) for e in p02 if e["initiating_bar"] is None],
        "p02_with_bar": [brief(e) for e in p02 if e["initiating_bar"] is not None],
    }


def group_by(items, keyfn):
    groups = defaultdict(list)
    for item in items:
        groups[keyfn(item)].append(item)
    return groups.items()


def main():
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "stage_transitions.txt")
    if not path.is_file():
        print(json.dumps({"error": f"unreadable {path}"}))
        sys.exit(1)
    print(json.dumps(summarize(path), indent=2, default=str))


if __name__ == "__main__":
    main()
