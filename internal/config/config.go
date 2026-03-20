package config

import (
	"fmt"
	"os"

	"github.com/Guy2co/algo-crypto-trader-bot/pkg/logger"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	Exchange  ExchangeConfig  `yaml:"exchange"`
	Bybit     BybitConfig     `yaml:"bybit"`
	Strategy  StrategyConfig  `yaml:"strategy"`
	Grid      GridConfig      `yaml:"grid"`
	Arbitrage ArbitrageConfig `yaml:"arbitrage"`
	Risk      RiskConfig      `yaml:"risk"`
	Backtest  BacktestConfig  `yaml:"backtest"`
	Logging   logger.Config   `yaml:"logging"`
	State     StateConfig     `yaml:"state"`
}

// ExchangeConfig holds exchange connectivity settings.
type ExchangeConfig struct {
	Name                   string `yaml:"name"`
	Testnet                bool   `yaml:"testnet"`
	RESTTimeoutSecs        int    `yaml:"rest_timeout_secs"`
	WSReconnectMaxAttempts int    `yaml:"ws_reconnect_max_attempts"`
}

// StrategyConfig selects the active strategy.
type StrategyConfig struct {
	Active string `yaml:"active"`
}

// GridConfig holds all grid trading parameters.
type GridConfig struct {
	Symbol          string  `yaml:"symbol"`
	QuoteAsset      string  `yaml:"quote_asset"`
	BaseAsset       string  `yaml:"base_asset"`
	GridBottom      float64 `yaml:"grid_bottom"`
	GridTop         float64 `yaml:"grid_top"`
	GridCount       int     `yaml:"grid_count"`
	TotalInvestment float64 `yaml:"total_investment"`
	FeeRate         float64 `yaml:"fee_rate"`
}

// RiskConfig holds all risk management parameters.
type RiskConfig struct {
	MaxPositionUSDT   float64 `yaml:"max_position_usdt"`
	StopLossPct       float64 `yaml:"stop_loss_pct"`
	MaxDrawdownPct    float64 `yaml:"max_drawdown_pct"`
	MaxOpenOrders     int     `yaml:"max_open_orders"`
	OrderCooldownSecs int     `yaml:"order_cooldown_secs"`
	CancelOnStop      bool    `yaml:"cancel_on_stop"`
}

// BacktestConfig holds backtesting parameters.
type BacktestConfig struct {
	DataDir             string  `yaml:"data_dir"`
	Symbol              string  `yaml:"symbol"`
	Interval            string  `yaml:"interval"`
	StartDate           string  `yaml:"start_date"`
	EndDate             string  `yaml:"end_date"`
	InitialBalanceUSDT  float64 `yaml:"initial_balance_usdt"`
	FeeRate             float64 `yaml:"fee_rate"`
}

// BybitConfig holds Bybit exchange connectivity settings.
type BybitConfig struct {
	Testnet         bool `yaml:"testnet"`
	RESTTimeoutSecs int  `yaml:"rest_timeout_secs"`
}

// ArbitrageConfig holds all arbitrage strategy parameters.
type ArbitrageConfig struct {
	Type                string   `yaml:"type"`                 // "triangular" | "cross_exchange" | "both"
	QuoteAssets         []string `yaml:"quote_assets"`         // e.g. ["USDT"]
	IntermediateAssets  []string `yaml:"intermediate_assets"`  // e.g. ["BTC","ETH","BNB","SOL"]
	MaxHops             int      `yaml:"max_hops"`             // 3 = triangular, 4 = quad
	MinProfitPct        float64  `yaml:"min_profit_pct"`       // 0.15 = 0.15%
	MaxTradeUSDT        float64  `yaml:"max_trade_usdt"`
	FeeRate             float64  `yaml:"fee_rate"`
	ScanIntervalMS      int      `yaml:"scan_interval_ms"`
	OrderTimeoutSecs    int      `yaml:"order_timeout_secs"`
	DryRun              bool     `yaml:"dry_run"`
}

// StateConfig holds state persistence settings.
type StateConfig struct {
	Dir                string `yaml:"dir"`
	FlushIntervalSecs  int    `yaml:"flush_interval_secs"`
}

// Load reads config from the given YAML file path, then overrides
// API keys from environment variables.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var cfg Config
	if err = yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return &cfg, nil
}
