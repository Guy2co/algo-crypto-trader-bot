package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/bot"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/config"
	binanceexchange "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
	binanceclient "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange/binance"
	bybitclient "github.com/Guy2co/algo-crypto-trader-bot/internal/exchange/bybit"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/risk"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy"
	"github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/arbitrage"
	"github.com/Guy2co/algo-crypto-trader-bot/pkg/logger"

	// Register strategies.
	_ "github.com/Guy2co/algo-crypto-trader-bot/internal/strategy/arbitrage/register"
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

	primaryEx, err := binanceclient.NewClient(apiKey, secretKey, cfg.Exchange.Testnet, log)
	if err != nil {
		return fmt.Errorf("init exchange: %w", err)
	}

	strat, err := buildStrategy(cfg, primaryEx, log)
	if err != nil {
		return fmt.Errorf("init strategy: %w", err)
	}

	riskMgr := risk.New(cfg.Risk, log)
	b := bot.New(cfg, primaryEx, strat, riskMgr, log)

	log.Info("starting algo trading bot",
		zap.String("strategy", cfg.Strategy.Active),
		zap.Bool("testnet", cfg.Exchange.Testnet),
	)

	return b.Run(context.Background())
}

// buildStrategy creates the active strategy, optionally wiring a second exchange for arbitrage.
func buildStrategy(cfg *config.Config, primaryEx binanceexchange.Exchange, log *zap.Logger) (strategy.Strategy, error) {
	if cfg.Strategy.Active != "arbitrage" {
		return strategy.New(cfg.Strategy.Active, cfg, log)
	}

	exchanges := []binanceexchange.Exchange{primaryEx}
	if bybitKey := os.Getenv("BYBIT_API_KEY"); bybitKey != "" {
		bybitEx, err := bybitclient.NewClient(bybitKey, os.Getenv("BYBIT_SECRET_KEY"), cfg.Bybit.Testnet, log)
		if err != nil {
			return nil, fmt.Errorf("init bybit exchange: %w", err)
		}
		exchanges = append(exchanges, bybitEx)
		log.Info("bybit exchange enabled for cross-exchange arbitrage")
	}

	return arbitrage.New(cfg, exchanges, log), nil
}
