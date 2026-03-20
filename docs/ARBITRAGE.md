# Arbitrage Strategy

> **Status**: Production-ready, `dry_run: true` by default. Switch to live after testnet verification.

---

## Table of Contents

1. [Strategy Overview](#strategy-overview)
2. [How Triangular Arbitrage Works](#how-triangular-arbitrage-works)
3. [How Quad-Arb Works](#how-quad-arb-works)
4. [How Cross-Exchange Arbitrage Works](#how-cross-exchange-arbitrage-works)
5. [Configuration Reference](#configuration-reference)
6. [Exchange Recommendations](#exchange-recommendations)
7. [Expected Returns](#expected-returns)
8. [Risk Management](#risk-management)
9. [Running in Dry-Run Mode](#running-in-dry-run-mode)
10. [Switching to Live Trading](#switching-to-live-trading)

---

## Strategy Overview

The arbitrage strategy exploits price inefficiencies between related assets or between exchanges. Three patterns are implemented:

| Pattern | Exchanges | Hops | Min Gross Profit Needed |
|---|---|---|---|
| **Triangular** | 1 (Binance) | 3 | 0.30% (3 × 0.1% fee) |
| **Quad-Arb** | 1 (Binance) | 4 | 0.40% (4 × 0.1% fee) |
| **Cross-Exchange** | 2 (Binance + Bybit) | 2 | 0.20% (2 × 0.1% fee) |

All three modes can be run simultaneously with `type: "both"` (triangular + quad + cross-exchange).

---

## How Triangular Arbitrage Works

Triangular arbitrage exploits cross-rate mispricing within a single exchange. When three assets have inconsistent exchange rates, a profit cycle exists.

### Example

```
USDT → BTC → ETH → USDT

Step 1: Buy  BTCUSDT  ask = 90,000  →  1 USDT buys 0.00001111 BTC
Step 2: Buy  ETHBTC   ask = 0.0667  →  0.00001111 BTC buys 0.0001664 ETH
Step 3: Sell ETHUSDT  bid = 6,010   →  0.0001664 ETH sells for 1.00026 USDT

Gross profit: +0.026%   Net after 3×0.1% fees: +0.026% − 0.30% = −0.274%  (unprofitable)
```

For an opportunity to be profitable, the gross multiplier must exceed `1 + 3×fee_rate + min_profit_pct`.

### Multiplier Formula

```
multiplier = product of all leg exchange rates

BUY  leg: rate = 1 / ask_price   (we spend quote to get base)
SELL leg: rate = bid_price        (we spend base to get quote)

net_profit_pct = (multiplier × (1 − fee_rate)^N − 1) × 100
```

### Paths Scanned (with BTC, ETH, BNB, SOL)

With 4 intermediate assets the scanner generates **C(4,2) × 2 × 1 quote = 12 triangular paths**:

```
USDT → BTC → ETH → USDT     USDT → ETH → BTC → USDT
USDT → BTC → BNB → USDT     USDT → BNB → BTC → USDT
USDT → BTC → SOL → USDT     USDT → SOL → BTC → USDT
USDT → ETH → BNB → USDT     USDT → BNB → ETH → USDT
USDT → ETH → SOL → USDT     USDT → SOL → ETH → USDT
USDT → BNB → SOL → USDT     USDT → SOL → BNB → USDT
```

---

## How Quad-Arb Works

Quad-arb is a 4-hop variant that traverses one additional intermediate asset. This opens more path combinations at the cost of a higher fee threshold.

```
USDT → BTC → ETH → BNB → USDT   (4 legs, 4 × 0.1% = 0.40% min cost)
```

Enable with `max_hops: 4` in config.

With 4 intermediates: **4! / 2 = 12 quad paths** per quote asset.

---

## How Cross-Exchange Arbitrage Works

Cross-exchange arb buys an asset cheaply on one exchange and sells it at a higher price on another, profiting from inter-exchange price divergence.

### Prerequisite: Pre-Funded Capital

Both exchanges must hold capital **before** trading begins:

| Exchange | Capital |
|---|---|
| Binance | USDT (to buy with) |
| Bybit | BTC/ETH/etc. (to sell with) |

No asset is transferred between exchanges during execution — both legs execute simultaneously on their respective exchanges.

### Example

```
Binance BTCUSDT ask = 90,000
Bybit   BTCUSDT bid = 90,400

Spread = (90400 − 90000) / 90000 = 0.44%
After 2 × 0.1% fees = 0.44% − 0.20% = 0.24% net profit  ✓
```

### Enabling Cross-Exchange Mode

1. Set `BYBIT_API_KEY` and `BYBIT_SECRET_KEY` environment variables.
2. Set `arbitrage.type: "cross_exchange"` or `"both"` in config.
3. Fund both exchange accounts.

---

## Configuration Reference

```yaml
# configs/config.yaml

bybit:
  testnet: false            # true = Bybit testnet
  rest_timeout_secs: 10

arbitrage:
  type: "triangular"        # "triangular" | "cross_exchange" | "both"
  quote_assets:             # currencies to start/end each cycle
    - "USDT"
  intermediate_assets:      # assets used in cycles
    - "BTC"
    - "ETH"
    - "BNB"
    - "SOL"
  max_hops: 3               # 3 = triangular only, 4 = add quad-arb paths
  min_profit_pct: 0.15      # minimum net profit % to trigger execution
  max_trade_usdt: 500.0     # maximum USDT budget per cycle
  fee_rate: 0.001           # taker fee rate (0.1% = 0.001)
  scan_interval_ms: 200     # how often to scan for opportunities (ms)
  order_timeout_secs: 5     # per-leg order placement timeout
  dry_run: true             # true = log only, false = execute orders
```

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `BINANCE_API_KEY` | Yes | Binance API key |
| `BINANCE_SECRET_KEY` | Yes | Binance secret key |
| `BYBIT_API_KEY` | No | Bybit API key (enables cross-exchange mode) |
| `BYBIT_SECRET_KEY` | No | Bybit secret key |

---

## Exchange Recommendations

### Primary: Binance Spot

- Highest liquidity globally → tighter spreads → more arb opportunities
- Taker fee: 0.10% (0.075% with BNB discount)
- WebSocket market data latency: ~2ms
- REST API rate limit: 1200 requests/min (weight-based)

### Secondary: Bybit Spot

- Different market microstructure (Asia-Pacific focus) → price discovery lag
- Taker fee: 0.10%
- Most useful for BTC, ETH, BNB, SOL cross-exchange pairs
- Requires pre-funded USDT on Binance + crypto on Bybit (or vice-versa)

### Why Not CEX → CEX Withdrawal

Withdrawals take 10–30 minutes and cost network fees. Cross-exchange arb requires **both sides to be pre-funded** so both legs execute simultaneously (no asset movement between exchanges).

---

## Expected Returns

These are order-of-magnitude estimates based on historical market microstructure data. Actual returns vary with market conditions.

| Mode | Frequency | Net Profit/Cycle | Annual Return (est.) |
|---|---|---|---|
| Triangular | 0–5 per day | 0.15–0.30% | 0–2% on deployed capital |
| Quad-Arb | 0–2 per day | 0.15–0.25% | 0–1% |
| Cross-Exchange | 1–20 per day | 0.20–0.50% | 2–10% |

**Important caveats:**
- Opportunities last milliseconds; HFT firms front-run retail arbitrageurs.
- Flash crashes and exchange outages create false positives.
- Start small (`max_trade_usdt: 100`) and scale after observing live behavior.
- `dry_run: true` is strongly recommended for the first 1–2 weeks.

---

## Risk Management

### Partial Failure (Stuck Position)

If leg 2 of a 3-leg cycle fails, the bot holds an intermediate asset (e.g. BTC) with no matching exit. The strategy logs the stuck position and **does not attempt recovery automatically**.

**Manual recovery:** Sell the intermediate asset at market price on the primary exchange.

Stuck positions are logged as:
```json
{"level":"error","msg":"arbitrage cycle failed","failed_leg":1,"error":"..."}
```

State is saved to `./state/triangular-arb.json` after every cycle. The `FailedLeg` index tells you exactly where execution stopped.

### Exchange Outage / API Timeout

Each leg has a configurable `order_timeout_secs` context. If the exchange API does not respond in time, the leg fails and the cycle is aborted. The bot logs the error and continues to the next scan interval — it does **not** crash.

### Slippage

Market orders are used for all legs. At low liquidity, fill price may differ significantly from the book price used to evaluate profitability. Increase `min_profit_pct` if you observe frequent negative-profit cycles.

### Fee Rate

Set `fee_rate` to match your actual taker fee. If you pay 0.075% (with BNB rebate on Binance), use `fee_rate: 0.00075`. Using a lower fee rate than actual will make unprofitable opportunities look profitable.

---

## Running in Dry-Run Mode

Dry-run mode scans for opportunities and logs them without placing any orders.

```yaml
# configs/config.yaml
strategy:
  active: "arbitrage"

arbitrage:
  type: "triangular"
  dry_run: true        # ← safe: no orders placed
  min_profit_pct: 0.15
```

```bash
export BINANCE_API_KEY=your_key
export BINANCE_SECRET_KEY=your_secret

go run ./cmd/bot -config configs/config.yaml
```

You will see log lines like:
```json
{"level":"info","msg":"arbitrage opportunity found","type":"triangular","net_profit_pct":0.18,"legs":3}
{"level":"info","msg":"arbitrage tick","cycles":0,"executed":0,"opportunities":42,"profit_usdt":0}
```

Run for at least **1–2 weeks** and count how many opportunities are found per day to calibrate `min_profit_pct` and `max_trade_usdt`.

---

## Switching to Live Trading

Follow this checklist before setting `dry_run: false`:

- [ ] Ran dry-run for ≥ 1 week and verified opportunity frequency
- [ ] Tested on **Binance testnet** first (`exchange.testnet: true`, `bybit.testnet: true`)
- [ ] Set `max_trade_usdt` conservatively (start at 100 USDT)
- [ ] Set `min_profit_pct` ≥ 0.20% (higher threshold = fewer but safer trades)
- [ ] Confirmed API key permissions: **Spot Trading only** (no withdrawal permission)
- [ ] Pre-funded Bybit with crypto if using cross-exchange mode
- [ ] Reviewed `./state/triangular-arb.json` for any stuck positions from dry run
- [ ] Set up alerting on `"arbitrage cycle failed"` log events

```yaml
arbitrage:
  dry_run: false
  min_profit_pct: 0.20
  max_trade_usdt: 100.0
  fee_rate: 0.001
```

```bash
export BINANCE_API_KEY=your_live_key
export BINANCE_SECRET_KEY=your_live_secret
# export BYBIT_API_KEY=your_bybit_key     # only for cross_exchange mode
# export BYBIT_SECRET_KEY=your_bybit_secret

go run ./cmd/bot -config configs/config.yaml
```

---

## State Persistence

Strategy state is saved to `./state/triangular-arb.json` after every executed cycle and on graceful stop. On restart the bot resumes from saved state (total cycles, profit, run ID).

```json
{
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "metrics": {
    "total_cycles": 7,
    "total_profit": 3.41,
    "total_fees_paid": 0.0,
    "total_opportunities": 1240,
    "total_executed": 7,
    "start_time": "2024-06-01T10:00:00Z",
    "last_cycle_time": "2024-06-01T14:32:11Z"
  }
}
```
