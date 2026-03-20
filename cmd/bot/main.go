package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	binanceexchange "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange/binance"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/risk"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/bot"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	"github.com/Guy2co/algo-crypto-trader-bot/pkg/logger"

	// Register strategies.
	_ "github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/grid/register"

	"go.uber.org/zap"
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

	apiKey := os.Getenv("BINANCE_API_KEY")
	secretKey := os.Getenv("BINANCE_SECRET_KEY")
	if apiKey == "" || secretKey == "" {
		return fmt.Errorf("BINANCE_API_KEY and BINANCE_SECRET_KEY env vars must be set")
	}

	ex, err := binanceexchange.NewClient(apiKey, secretKey, cfg.Exchange.Testnet, log)
	if err != nil {
		return fmt.Errorf("init exchange: %w", err)
	}

	strat, err := strategy.New(cfg.Strategy.Active, cfg, log)
	if err != nil {
		return fmt.Errorf("init strategy: %w", err)
	}

	riskMgr := risk.New(cfg.Risk, log)

	b := bot.New(cfg, ex, strat, riskMgr, log)

	log.Info("starting algo trading bot",
		zap.String("strategy", cfg.Strategy.Active),
		zap.String("symbol", cfg.Grid.Symbol),
		zap.Bool("testnet", cfg.Exchange.Testnet),
	)

	return b.Run(context.Background())
}
