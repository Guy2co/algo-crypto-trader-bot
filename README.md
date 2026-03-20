# algo-crypto-trader-bot

A modular 24/7 crypto algorithmic trading bot written in Go, targeting **Binance Spot**.

**Current strategy: Grid Trading** — places buy/sell limit orders at fixed price intervals and profits from price oscillation. Designed for 24/7 crypto markets with no market close.

## Strategy Overview: Grid Trading

| Metric | Typical Range |
|--------|--------------|
| Win Rate | 70–90% |
| Sharpe Ratio | 1.2–2.0+ |
| Max Drawdown | 5–12% |
| Best Markets | Ranging / sideways |

The bot divides a price range (e.g. $80,000–$100,000) into N equal intervals, places buy orders below the current price and sell orders above. When a buy fills, a sell is placed one level higher — capturing the spread as profit on each round trip.

---

## Prerequisites

- Go 1.23+
- Binance account with Spot trading enabled
- For testnet: register at [testnet.binance.vision](https://testnet.binance.vision)

---

## Configuration

Copy and edit the config file:

```bash
cp configs/config.yaml configs/config.local.yaml
```

Key parameters to set:

```yaml
grid:
  symbol: "BTCUSDT"
  grid_bottom: 80000.0   # lower bound of grid range
  grid_top: 100000.0     # upper bound of grid range
  grid_count: 20         # number of grid intervals
  total_investment: 1000.0  # USDT to deploy
```

**API keys are never stored in config files.** Set them as environment variables:

```bash
export BINANCE_API_KEY=your_key
export BINANCE_SECRET_KEY=your_secret
```

---

## Running the Backtest

1. Place a BTCUSDT OHLCV CSV file in `./data/BTCUSDT-1h.csv`

   Expected format (columns, unix milliseconds for timestamps):
   ```
   open_time,open,high,low,close,volume,close_time
   1704067200000,42000.0,43000.0,41500.0,42500.0,1234.5,1704070799999
   ```

   You can download historical data from [Binance Vision](https://data.binance.vision) or use the Binance REST API.

2. Configure the backtest parameters in `configs/config.yaml`:

   ```yaml
   backtest:
     symbol: "BTCUSDT"
     interval: "1h"
     start_date: "2024-01-01"
     end_date:   "2024-12-31"
     initial_balance_usdt: 10000.0
   ```

3. Run the backtest:

   ```bash
   make backtest
   # or with a custom config:
   make backtest CONFIG=configs/config.local.yaml
   ```

   Example output:
   ```
   ============================================================
     Backtest Report — BTCUSDT
   ============================================================
     Period       : 2024-01-01 → 2024-12-31
     Initial USDT : 10000.00
     Final USDT   : 11842.30
   ------------------------------------------------------------
     Total Return : 18.42%
     Ann. Return  : 18.42%
     Max Drawdown : 7.23%
     Sharpe Ratio : 1.41
   ------------------------------------------------------------
     Total Trades : 847
     Cycles Done  : 423
     Fees Paid    : 38.1200 USDT
   ============================================================
   ```

---

## Running on Testnet (Paper Trading)

1. Register at [testnet.binance.vision](https://testnet.binance.vision) and generate API keys.

2. Set testnet keys:
   ```bash
   export BINANCE_API_KEY=your_testnet_key
   export BINANCE_SECRET_KEY=your_testnet_secret
   ```

3. Enable testnet in config:
   ```bash
   cp configs/config.yaml configs/config.testnet.yaml
   ```
   Edit `configs/config.testnet.yaml`:
   ```yaml
   exchange:
     testnet: true

   grid:
     # Adjust grid range to current testnet BTC price
     grid_bottom: 80000.0
     grid_top: 100000.0
   ```

4. Run:
   ```bash
   make run CONFIG=configs/config.testnet.yaml
   ```

5. Verify orders appear in the [Binance Testnet UI](https://testnet.binance.vision/en/trade/BTC_USDT).

6. Stop with `Ctrl+C` — the bot saves state to `./state/BTCUSDT-grid.json` and shuts down gracefully.

7. **Test recovery:** restart the bot and confirm it reconciles existing orders without duplicating them.

---

## Running Live

```bash
export BINANCE_API_KEY=your_live_key
export BINANCE_SECRET_KEY=your_live_secret

# Ensure exchange.testnet: false in config
make run CONFIG=configs/config.yaml
```

> **Warning:** Live trading uses real funds. Always test on testnet first and start with a small investment.

---

## Risk Management

The bot enforces these controls automatically:

| Control | Default | Behaviour |
|---------|---------|-----------|
| `stop_loss_pct: 5.0` | 5% outside grid | Halts the bot |
| `max_drawdown_pct: 15.0` | 15% from equity peak | Halts the bot |
| `max_open_orders: 50` | 50 orders | Blocks new orders |
| `max_position_usdt: 1100.0` | USDT at risk cap | Blocks new orders |
| `cancel_on_stop: false` | Leave orders on halt | Set `true` to cancel |

---

## Makefile Targets

```bash
make run        # start live trading bot
make backtest   # run backtest simulation
make test       # go test ./... -count=1
make build      # compile binaries → ./bin/
make lint       # golangci-lint run ./...
make tidy       # go mod tidy
```

---

## Project Structure

```
cmd/
  bot/main.go          # live trading entry point
  backtest/main.go     # backtest entry point
internal/
  config/              # YAML config loading + validation
  exchange/
    exchange.go        # Exchange interface + shared types
    binance/           # Binance REST + WebSocket implementation
    mock/              # In-memory mock for testing + backtesting
  strategy/
    grid/              # Grid trading strategy
  risk/                # Pre-trade + portfolio risk controls
  order/               # Fill deduplication tracker
  backtest/            # Candle-driven simulation engine
  bot/                 # Top-level coordinator + event loop
pkg/logger/            # Zap logger initialisation
configs/config.yaml    # Default configuration
docs/PLAN.md           # Architecture + design rationale
```

---

## Adding a New Strategy

1. Create `internal/strategy/yourname/yourname.go` implementing `strategy.Strategy`
2. Create `internal/strategy/yourname/register/register.go` calling `strategy.Register("yourname", ...)`
3. Blank-import the register package in `cmd/bot/main.go` and `cmd/backtest/main.go`
4. Set `strategy.active: "yourname"` in `configs/config.yaml`
