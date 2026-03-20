package config

import (
	"errors"
	"fmt"
)

// Validate checks that the config is complete and internally consistent.
func (c *Config) Validate() error {
	var errs []error

	if c.Exchange.Name == "" {
		errs = append(errs, errors.New("exchange.name is required"))
	}
	if c.Strategy.Active == "" {
		errs = append(errs, errors.New("strategy.active is required"))
	}

	if err := c.Grid.validate(); err != nil {
		errs = append(errs, fmt.Errorf("grid: %w", err))
	}
	if err := c.Risk.validate(); err != nil {
		errs = append(errs, fmt.Errorf("risk: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (g GridConfig) validate() error {
	var errs []error
	if g.Symbol == "" {
		errs = append(errs, errors.New("symbol is required"))
	}
	if g.GridBottom <= 0 {
		errs = append(errs, errors.New("grid_bottom must be > 0"))
	}
	if g.GridTop <= g.GridBottom {
		errs = append(errs, errors.New("grid_top must be > grid_bottom"))
	}
	if g.GridCount < 2 {
		errs = append(errs, errors.New("grid_count must be >= 2"))
	}
	if g.TotalInvestment <= 0 {
		errs = append(errs, errors.New("total_investment must be > 0"))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (r RiskConfig) validate() error {
	var errs []error
	if r.MaxPositionUSDT <= 0 {
		errs = append(errs, errors.New("max_position_usdt must be > 0"))
	}
	if r.MaxOpenOrders <= 0 {
		errs = append(errs, errors.New("max_open_orders must be > 0"))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
