package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/backtest"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/pkg/logger"

	// Register strategies.
	_ "github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/grid/register"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	log, err := logger.New(cfg.Logging)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer log.Sync() //nolint:errcheck

	csvPath := backtest.DataPath(cfg.Backtest.DataDir, cfg.Backtest.Symbol, cfg.Backtest.Interval)
	feed, err := backtest.LoadCSV(csvPath)
	if err != nil {
		return fmt.Errorf("load candle data from %s: %w", csvPath, err)
	}

	strat, err := strategy.New(cfg.Strategy.Active, cfg, log)
	if err != nil {
		return fmt.Errorf("init strategy: %w", err)
	}

	engine := backtest.NewEngine(cfg, strat, feed, log)
	report, err := engine.Run(context.Background())
	if err != nil {
		return fmt.Errorf("run backtest: %w", err)
	}

	report.Print()
	return nil
}
