// Package backtest provides a candle-driven simulation engine for strategy testing.
package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

// CandleFeed loads OHLCV candle data from a CSV file.
//
// Expected CSV columns (no header required, but header is tolerated):
//
//	open_time (unix ms), open, high, low, close, volume, close_time (unix ms)
type CandleFeed struct {
	candles []exchange.Candle
}

// LoadCSV reads candle data from a CSV file.
func LoadCSV(path string) (*CandleFeed, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("open candle CSV %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	r := csv.NewReader(f)
	var candles []exchange.Candle

	for {
		row, readErr := r.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read CSV row: %w", readErr)
		}
		if len(row) < 7 {
			continue
		}
		// Skip header row if present.
		if row[0] == "open_time" || row[0] == "Open time" {
			continue
		}

		c, parseErr := parseRow(row)
		if parseErr != nil {
			return nil, fmt.Errorf("parse row %v: %w", row, parseErr)
		}
		candles = append(candles, c)
	}

	return &CandleFeed{candles: candles}, nil
}

// DataPath builds the default CSV path for a symbol and interval.
func DataPath(dataDir, symbol, interval string) string {
	return filepath.Join(dataDir, symbol+"-"+interval+".csv")
}

func parseRow(row []string) (exchange.Candle, error) {
	openTimeMs, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("open_time: %w", err)
	}
	open, err := strconv.ParseFloat(row[1], 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("open: %w", err)
	}
	high, err := strconv.ParseFloat(row[2], 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("high: %w", err)
	}
	low, err := strconv.ParseFloat(row[3], 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("low: %w", err)
	}
	close_, err := strconv.ParseFloat(row[4], 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("close: %w", err)
	}
	volume, err := strconv.ParseFloat(row[5], 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("volume: %w", err)
	}
	closeTimeMs, err := strconv.ParseInt(row[6], 10, 64)
	if err != nil {
		return exchange.Candle{}, fmt.Errorf("close_time: %w", err)
	}

	return exchange.Candle{
		OpenTime:  time.UnixMilli(openTimeMs),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close_,
		Volume:    volume,
		CloseTime: time.UnixMilli(closeTimeMs),
	}, nil
}

// Candles returns the loaded candle slice.
func (f *CandleFeed) Candles() []exchange.Candle {
	return f.candles
}
