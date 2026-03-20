# Crypto Algo Trading Bot — Grid Trading on Binance Spot (Go)

## Context

Brand new project (`/home/ubuntu/code/algo-trade` is empty). The user wants a 24/7 crypto algo trading bot in **Go** targeting **Binance Spot**, starting with **Grid Trading** as the primary strategy. Grid trading was chosen as the most suitable strategy for 24/7 crypto markets: it profits from price oscillation within a range, achieves 70–90% win rates with Sharpe ratios of 1.2–2.0+, and keeps capital continuously deployed. The architecture must be modular to support additional strategies later.

---

## Project Structure

```
algo-trade/
├── cmd/
│   ├── bot/main.go              # Live trading entry point, CLI flags, wiring
│   └── backtest/main.go         # Backtest runner entry point
├── internal/
│   ├── config/
│   │   ├── config.go            # Config structs + YAML loader + env var overrides
│   │   └── validate.go          # Fatal validation (missing keys, bad ranges)
│   ├── exchange/
│   │   ├── exchange.go          # Exchange interface + shared types (Order, Fill, Balance, Candle)
│   │   ├── binance/
│   │   │   ├── client.go        # REST wrapper (PlaceLimitOrder, GetOpenOrders, etc.)
│   │   │   ├── stream.go        # WebSocket user data stream + listenKey keepalive + reconnect
│   │   │   ├── mapper.go        # Binance SDK types → internal types
│   │   │   └── errors.go        # Typed error handling + retry on 5xx
│   │   └── mock/
│   │       └── exchange.go      # In-memory mock for backtesting (SimulateFills per candle)
│   ├── strategy/
│   │   ├── strategy.go          # Strategy interface (Init, OnFill, OnTick, Stop)
│   │   ├── registry.go          # Factory map: "grid" → constructor
│   │   └── grid/
│   │       ├── grid.go          # Orchestrator: implements Strategy interface
│   │       ├── state.go         # GridState + GridLevel structs, JSON persistence
│   │       ├── calculator.go    # ComputeLevels, ComputeQuantityPerGrid, RoundToTickSize
│   │       └── orders.go        # InitializeGrid, RecoverGrid, HandleFill (core logic)
│   ├── risk/
│   │   ├── manager.go           # RiskManager interface + Manager struct
│   │   └── checks.go            # CheckOrderPlacement, CheckStopLoss, CheckDrawdown
│   ├── order/
│   │   ├── types.go             # Internal Order, Fill types
│   │   └── tracker.go           # In-memory TradeID dedup set (prevents double-fill handling)
│   ├── backtest/
│   │   ├── engine.go            # Candle-driven simulation loop
│   │   ├── feed.go              # Load/download + cache historical OHLCV CSVs
│   │   └── report.go            # PnL metrics: Sharpe, CAGR, max drawdown, win rate
│   └── bot/
│       └── bot.go               # Bot struct: Run() with event loop (select on fills/tick/signal)
├── pkg/logger/
│   └── logger.go                # Zap logger init (JSON prod / console dev)
├── configs/
│   └── config.yaml              # Default config (API keys via env vars only)
├── data/.gitkeep                # Historical candle CSVs (gitignored)
├── state/.gitkeep               # Runtime JSON state (gitignored)
├── Makefile                     # run / backtest / test / build / lint targets
├── go.mod
└── go.sum
```

---

## Key Interfaces

### exchange.Exchange
Central abstraction — all strategy and risk code depends only on this interface:
- `PlaceLimitOrder`, `CancelOrder`, `CancelAllOrders`, `GetOrder`, `GetOpenOrders`
- `GetBalances`, `GetCurrentPrice`, `GetCandles`
- `SubscribeOrderFills(ctx) (<-chan OrderFillEvent, CancelFunc, error)` — WebSocket stream
- `FormatPrice`, `FormatQuantity` — exchange-specific tick/step size rounding

### strategy.Strategy
Lifecycle interface all strategies implement:
- `Init(ctx, ex)` — load or create state, place/recover initial orders
- `OnFill(ctx, ex, event)` — core rebalancing hook
- `OnTick(ctx, ex)` — periodic housekeeping (60s interval)
- `Stop(ctx, ex)` — graceful shutdown, optional order cancel, flush state

### risk.RiskManager
- `CheckOrderPlacement(req, openOrders, balances)` — enforces all pre-order checks
- `CheckStopLoss(currentPrice, gridBottom, gridTop)` — halts on range breach
- `CheckDrawdown(currentEquity)` — halts on equity drawdown from peak
- `RecordEquity(equity)` — updates high-water mark

---

## Grid Trading Logic

### Initialization (no prior state)
1. `calculator.ComputeLevels(bottom, top, count)` → N+1 evenly-spaced price levels
2. `ComputeQuantityPerGrid(investment, count, midPrice)` → qty per grid slot
3. Levels below `currentPrice` → place **BUY** limit orders
4. Levels above `currentPrice` → place **SELL** limit orders
5. Store all `OrderID`s in `GridState.Levels[i]`, persist to `state/{symbol}-grid.json`

### Recovery (state file exists)
1. Load `GridState` from JSON
2. `GetOpenOrders` from exchange → build set of live `ClientOrderID`s
3. For each level: if expected order missing → re-place (idempotent via deterministic ClientOrderID)

### Fill Handling — `HandleFill` (highest-risk function)
- **BUY fills at level[i]**: place SELL at level[i+1], record profit contribution
- **SELL fills at level[i]**: place BUY at level[i-1], compute realized profit = `(sellPrice − buyPrice) × qty − fees`
- Edge guards: no order below level[0] or above level[N]
- Skip partial fills (`Status != FILLED`)
- Duplicate fills dropped by `order/tracker.go` TradeID set

### ClientOrderID convention
`"{symbol}-{runID[0:8]}-L{index:03d}-{BUY|SELL}"` — deterministic, survives restarts, enables idempotency on re-placement

---

## Binance Integration

### REST (`binance/client.go`)
- Wraps `github.com/adshao/go-binance/v2`
- On init: `loadSymbolInfo()` caches tick size + step size + min notional per symbol
- `PlaceLimitOrder`: formats qty/price, retries on HTTP 5xx (3× exponential backoff), maps response via `mapper.go`

### WebSocket (`binance/stream.go`)
1. POST `/api/v3/userDataStream` → `listenKey`
2. Goroutine: PUT keepalive every 30 minutes
3. `binance.WsUserDataServe(listenKey, handler, errHandler)`
4. Filter `executionReport` events with `Status == FILLED`
5. On disconnect: reconnect with backoff (100ms → 200ms → … → 30s cap)
6. On ctx cancel: DELETE listenKey, close channel

---

## Configuration (`configs/config.yaml`)

```yaml
exchange:
  name: "binance"
  testnet: false          # set true for paper trading on testnet
  rest_timeout_secs: 10

strategy:
  active: "grid"          # registry key — swap to add future strategies

grid:
  symbol: "BTCUSDT"
  grid_bottom: 80000.0
  grid_top: 100000.0
  grid_count: 20          # 20 intervals = 21 price levels
  total_investment: 1000.0  # USDT
  fee_rate: 0.001          # 0.1% taker; use 0.00075 with BNB discount

risk:
  max_position_usdt: 1100.0
  stop_loss_pct: 5.0       # halt if price >5% outside grid range
  max_drawdown_pct: 15.0
  max_open_orders: 50
  order_cooldown_secs: 1
  cancel_on_stop: false    # leave orders + alert on stop-loss (safer default)

backtest:
  data_dir: "./data"
  symbol: "BTCUSDT"
  interval: "1h"
  start_date: "2024-01-01"
  end_date:   "2024-12-31"
  initial_balance_usdt: 10000.0
  fee_rate: 0.001

logging:
  level: "info"
  format: "json"
  output_path: "stdout"

state:
  dir: "./state"
  flush_interval_secs: 30
```

**API keys come exclusively from environment variables** (`BINANCE_API_KEY`, `BINANCE_SECRET_KEY`) — never stored in the config file.

---

## Risk Management

Checks run **before every order placement** and on every **tick + fill**:

| Check | Trigger | Action |
|---|---|---|
| `halted` flag | Any check fails | Blocks all order placement |
| Max open orders | `len(openOrders) >= max_open_orders` | Block order |
| Max position USDT | Sum of buy-side exposure > limit | Block order |
| Order cooldown | Time since last order < cooldown | Block order |
| Stop loss | Price >5% outside grid range | Set halted=true |
| Max drawdown | Equity drops >15% from peak | Set halted=true |

---

## Backtesting Engine

- `CandleFeed` loads OHLCV from CSV (or downloads via Binance REST and caches)
- Per candle: `MockExchange.SimulateFills(candle)` — BUY fills if `order.Price >= candle.Low`; SELL fills if `order.Price <= candle.High`
- Each simulated fill → `strategy.OnFill()` — **identical code path as live trading**
- `Report` outputs: total return, CAGR, Sharpe ratio, max drawdown, win rate, fees paid, completed cycles

---

## Startup Sequence (`bot.Run`)

1. Load + validate config; override API keys from env
2. Init logger (`zap`)
3. Init Binance client → `loadSymbolInfo` → `GetBalances` (connectivity check)
4. Init risk manager → compute + record initial equity
5. `strategy.Init` → recover or initialize grid
6. `exchange.SubscribeOrderFills` → start WebSocket + keepalive goroutine
7. Start 60s tick timer
8. **Event loop** (`select`):
   - `fillChan` → deduplicate TradeID → risk checks → `strategy.OnFill` → async state flush
   - `ticker.C` → `strategy.OnTick` → equity check → drawdown check
   - `ctx.Done` → `strategy.Stop` → cancel stream → exit
9. SIGINT/SIGTERM cancels ctx → 10s graceful drain

---

## Go Dependencies

```
github.com/adshao/go-binance/v2 v2.6.0
go.uber.org/zap v1.27.0
gopkg.in/yaml.v3 v3.0.1
github.com/google/uuid v1.6.0
```

---

## CI / Lint / Pre-commit (copied from expense-bot)

### `.github/workflows/ci.yml`
```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main, master]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go build ./...
      - run: go test ./...
      - uses: golangci/golangci-lint-action@v9
        with:
          version: latest
```

### `.golangci.yml`
Identical to expense-bot's config (20+ linters: staticcheck, gosec, errorlint, gocritic, cyclop, nestif, misspell, etc.). Key exclusions:
- `perfsprint` string-concat suggestions suppressed
- `gocritic exitAfterDefer` excluded for `cmd/*/main.go`
- `gosec G104` (errcheck dupe) and `G704` (SSRF false positive) excluded

### `.pre-commit-config.yaml`
- `gitleaks` + `trufflehog` for secret scanning
- `golangci-lint run ./...` local hook
- `go test ./...` local hook

---

## Unit Tests (one `_test.go` per package)

| File | What to test |
|---|---|
| `internal/strategy/grid/calculator_test.go` | `ComputeLevels` (count, spacing, boundary values), `ComputeQuantityPerGrid`, `RoundToTickSize`, `RoundToStepSize` |
| `internal/strategy/grid/orders_test.go` | `HandleFill` BUY side, SELL side, edge levels (0 and N), partial fill ignored, duplicate TradeID ignored |
| `internal/strategy/grid/state_test.go` | JSON marshal/unmarshal round-trip, mutex safety |
| `internal/risk/checks_test.go` | All three checks (order placement, stop loss, drawdown), halted flag propagation |
| `internal/order/tracker_test.go` | TradeID dedup (seen → dropped, unseen → passed) |
| `internal/exchange/mock/exchange_test.go` | `SimulateFills`: BUY fills when price touches low, SELL when touches high, fee deduction, balance update |
| `internal/backtest/engine_test.go` | Full simulation with synthetic candles; verify final equity > initial for a known grid scenario |
| `internal/exchange/binance/mapper_test.go` | Mapping correctness for Order and OrderFillEvent fields |

All tests use only the mock exchange — no real Binance calls in tests. Run with `go test ./... -race`.

---

## Repo Docs

### `docs/PLAN.md`
Copy of this implementation plan committed to the repo so the architecture rationale lives alongside the code.

### `README.md` — sections:

#### Prerequisites
```
Go 1.25+
Binance account (Spot trading enabled)
For testnet: register at testnet.binance.vision
```

#### Running the Backtest
```bash
# Download or place BTCUSDT 1h OHLCV CSV in ./data/BTCUSDT-1h.csv
# (columns: open_time, open, high, low, close, volume, close_time)

cp configs/config.yaml configs/config.local.yaml
# Edit config.local.yaml: set backtest.start_date, end_date, grid params

make backtest CONFIG=configs/config.local.yaml
# Output: Sharpe ratio, CAGR, max drawdown, win rate, completed cycles
```

#### Running on Testnet (Paper Trading)
```bash
# 1. Get testnet keys from https://testnet.binance.vision
export BINANCE_API_KEY=your_testnet_key
export BINANCE_SECRET_KEY=your_testnet_secret

# 2. Enable testnet in config
cp configs/config.yaml configs/config.testnet.yaml
# Set: exchange.testnet: true
# Set grid params (testnet BTC price may differ — adjust grid_bottom/top)

make run CONFIG=configs/config.testnet.yaml

# 3. Verify orders appear in Binance Testnet UI
# 4. Ctrl+C for graceful shutdown; state saved to ./state/BTCUSDT-grid.json
# 5. Restart to test recovery: make run CONFIG=configs/config.testnet.yaml
```

#### Running Live
```bash
export BINANCE_API_KEY=your_live_key
export BINANCE_SECRET_KEY=your_live_secret
# Ensure exchange.testnet: false in config
make run CONFIG=configs/config.yaml
```

#### Makefile targets
```
make run        # live bot
make backtest   # backtest simulation
make test       # go test ./... -race
make build      # compile binaries to ./bin/
make lint       # golangci-lint run ./...
```

---

## Verification

1. `go test ./... -race` — all unit tests green, no data races
2. `make backtest` on BTCUSDT 2024 1h candles → positive return, max drawdown <20%
3. Testnet run → orders visible in Binance testnet UI, fills trigger rebalancing (check logs)
4. Recovery test: kill bot mid-run, restart → `RecoverGrid` re-places only missing orders, no duplicates (verify via TradeID tracker)

---

## Publishing

After implementation and tests pass:

```bash
git init
git remote add origin https://github.com/Guy2co/algo-crypto-trader-bot.git
git add .
git commit -m "Initial implementation: grid trading bot for Binance Spot"
git push -u origin main
```

Repo: https://github.com/Guy2co/algo-crypto-trader-bot.git
