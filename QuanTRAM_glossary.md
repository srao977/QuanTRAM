# Use and Meaning of Bar
In the gaps document, **“bar” means a time-bucketed market-data observation**, commonly called an **OHLCV bar** or candlestick. It does not mean a chart component or an order-book level.

For a one-minute interval $[t, t+1\text{ minute})$, a trade-based bar usually contains:

$$
\begin{aligned}
O &= \text{first eligible trade price} \\
H &= \max(\text{eligible trade prices}) \\
L &= \min(\text{eligible trade prices}) \\
C &= \text{last eligible trade price} \\
V &= \sum(\text{eligible trade quantities})
\end{aligned}
$$

It should also carry metadata such as:

- Symbol and instrument ID
- Interval start and end
- Instrument type
- Data provider
- Event count
- Finalization and quality status
- Whether it was backfilled or crossed a provider transition
- A unique bar or market-snapshot ID

**How The Gap Document Uses It**

- **Canonical bar semantics:** The exact rules for deciding which trades contribute to `O`, `H`, `L`, `C`, and `V`. Alpaca and Databento may apply different filtering or correction rules.
- **Bar boundary:** The start and end of the interval. For example, a one-minute bar could represent `[14:30:00, 14:31:00)` UTC.
- **Open or partial bar:** The interval is still receiving events. Its values can still change and generally should not drive a close-of-bar strategy.
- **Provisional bar:** The interval has ended, but delayed events or corrections may still arrive.
- **Finalized bar:** QuanTRAM has passed its allowed-lateness threshold and declares the bar stable enough for inference.
- **Missing bar:** An expected interval has no acceptable completed observation. This is different from a valid zero-volume interval.
- **Backfilled bar:** A bar recovered from a historical endpoint after missing or interrupted live data.
- **Mid-bar failover:** QuanTRAM switches from Alpaca to Databento before the current interval is finalized.
- **Bar continuity:** Consecutive expected intervals exist, are ordered correctly, and have no unexplained duplicates or gaps.
- **Bar replay:** Reprocessing the same source events and configuration produces the same finalized bar.

**Provider Versus Local Bars**

The document distinguishes two possible sources:

1. **Provider-supplied bars:** Alpaca or Databento calculates OHLCV and sends the completed bar.
2. **Locally aggregated bars:** QuanTRAM consumes individual trades and calculates OHLCV itself.

QuanTRAM needs to decide which form is authoritative for each interval. Mixing a partial locally calculated bar with a provider-supplied replacement without reconciliation can distort high, low, close, and especially volume.

**Why Bars Matter To Decision Accuracy**

Bars are the direct inputs to many features:

- Returns and momentum
- Realized volatility
- Moving averages
- Volume anomalies
- Breakouts and ranges
- Regime classification

A malformed close changes returns. Missing volume changes liquidity features. A duplicate bar changes rolling-window alignment. A revised historical bar can introduce look-ahead bias. This is why the gaps document treats bar construction, finalization, provenance, and failover as P0 concerns.

**Important Precision**

Standard OHLCV should normally be calculated from **eligible trades**, not quotes. Bid/ask data may produce separate quote bars or spread features, but those should not silently be mixed into trade OHLCV.

Published indices also require special treatment. Their open, high, low, and close may be useful, but volume may be unavailable or provider-defined. QuanTRAM should represent unavailable index volume as nullable or explicitly unsupported, not automatically as zero.

Created 2 todos