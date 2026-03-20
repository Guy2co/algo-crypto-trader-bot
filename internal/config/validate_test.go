package config

import (
	"testing"
)

func validConfig() *Config {
	return &Config{
		Exchange: ExchangeConfig{Name: "binance"},
		Strategy: StrategyConfig{Active: "grid"},
		Grid: GridConfig{
			Symbol:          "BTCUSDT",
			GridBottom:      80000,
			GridTop:         100000,
			GridCount:       10,
			TotalInvestment: 1000,
		},
		Risk: RiskConfig{
			MaxPositionUSDT: 1100,
			MaxOpenOrders:   50,
		},
	}
}

func TestValidate_Valid(t *testing.T) {
	t.Parallel()
	if err := validConfig().Validate(); err != nil {
		t.Errorf("expected valid config to pass, got: %v", err)
	}
}

func TestValidate_MissingExchangeName(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Exchange.Name = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing exchange name")
	}
}

func TestValidate_GridBottomZero(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Grid.GridBottom = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero grid_bottom")
	}
}

func TestValidate_GridTopLessBottom(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Grid.GridTop = 70000
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when grid_top <= grid_bottom")
	}
}

func TestValidate_GridCountTooLow(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Grid.GridCount = 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for grid_count < 2")
	}
}

func TestValidate_MaxOpenOrdersZero(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Risk.MaxOpenOrders = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero max_open_orders")
	}
}
