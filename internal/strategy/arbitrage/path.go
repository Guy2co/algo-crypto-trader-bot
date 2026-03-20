// Package arbitrage implements triangular and cross-exchange arbitrage strategies.
package arbitrage

import (
	"context"
	"fmt"
	"math"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/exchange"
)

// Leg describes one trade in a multi-hop arbitrage path.
type Leg struct {
	Symbol   string
	Side     exchange.OrderSide
	Exchange exchange.Exchange
}

// ArbPath is a discovered profitable cycle of N legs.
type ArbPath struct {
	Legs         []Leg
	Tickers      []exchange.BookTicker
	// Multiplier > 1.0 means gross profit before fees.
	Multiplier   float64
	// NetProfitPct is expected profit % after all fees.
	NetProfitPct float64
	PathType     string // "triangular", "quad", "cross_exchange"
}

// pathTemplate is a static description of a triangle path (not yet priced).
type pathTemplate struct {
	legs     []Leg
	pathType string
}

// BuildTrianglePaths generates all candidate triangle paths for the given
// quote assets, intermediate assets, and exchanges.
//
// For each pair of intermediates (A, B) and a quote Q, two paths are created:
//
//	Forward:  Q→A→B→Q   (BUY A/Q, BUY B/A, SELL B/Q)
//	Reverse:  Q→B→A→Q   (BUY B/Q, SELL B/A, SELL A/Q)
func BuildTrianglePaths(quoteAssets, intermediates []string, exs []exchange.Exchange) []pathTemplate {
	primary := exs[0]
	var templates []pathTemplate

	for _, quote := range quoteAssets {
		for i := 0; i < len(intermediates); i++ {
			for j := i + 1; j < len(intermediates); j++ {
				a := intermediates[i]
				b := intermediates[j]

				symbolAQ := a + quote  // e.g. BTCUSDT
				symbolBA := b + a      // e.g. ETHBTC
				symbolBQ := b + quote  // e.g. ETHUSDT

				// Forward: Q→A (buy A/Q), A→B (buy B/A), B→Q (sell B/Q)
				templates = append(templates, pathTemplate{
					pathType: "triangular",
					legs: []Leg{
						{Symbol: symbolAQ, Side: exchange.OrderSideBuy, Exchange: primary},
						{Symbol: symbolBA, Side: exchange.OrderSideBuy, Exchange: primary},
						{Symbol: symbolBQ, Side: exchange.OrderSideSell, Exchange: primary},
					},
				})

				// Reverse: Q→B (buy B/Q), B→A (sell B/A), A→Q (sell A/Q)
				templates = append(templates, pathTemplate{
					pathType: "triangular",
					legs: []Leg{
						{Symbol: symbolBQ, Side: exchange.OrderSideBuy, Exchange: primary},
						{Symbol: symbolBA, Side: exchange.OrderSideSell, Exchange: primary},
						{Symbol: symbolAQ, Side: exchange.OrderSideSell, Exchange: primary},
					},
				})
			}
		}
	}
	return templates
}

// BuildQuadPaths generates 4-hop arbitrage paths (quad-arb).
// Example: USDT→BTC→ETH→BNB→USDT
func BuildQuadPaths(quoteAssets, intermediates []string, exs []exchange.Exchange) []pathTemplate {
	if len(intermediates) < 3 {
		return nil
	}
	primary := exs[0]
	var templates []pathTemplate

	for _, quote := range quoteAssets {
		for i := 0; i < len(intermediates); i++ {
			for j := 0; j < len(intermediates); j++ {
				if j == i {
					continue
				}
				for k := 0; k < len(intermediates); k++ {
					if k == i || k == j {
						continue
					}
					a := intermediates[i]
					b := intermediates[j]
					c := intermediates[k]
					// Q→A→B→C→Q
					templates = append(templates, pathTemplate{
						pathType: "quad",
						legs: []Leg{
							{Symbol: a + quote, Side: exchange.OrderSideBuy, Exchange: primary},
							{Symbol: b + a, Side: exchange.OrderSideBuy, Exchange: primary},
							{Symbol: c + b, Side: exchange.OrderSideBuy, Exchange: primary},
							{Symbol: c + quote, Side: exchange.OrderSideSell, Exchange: primary},
						},
					})
				}
			}
		}
	}
	return templates
}

// BuildCrossExchangePaths creates cross-exchange paths (buy on ex[0], sell on ex[1]).
// Requires len(exs) >= 2.
func BuildCrossExchangePaths(quoteAssets, intermediates []string, exs []exchange.Exchange) []pathTemplate {
	if len(exs) < 2 {
		return nil
	}
	var templates []pathTemplate
	for _, quote := range quoteAssets {
		for _, asset := range intermediates {
			symbol := asset + quote
			// Buy cheap on exchange[0], sell expensive on exchange[1].
			templates = append(templates, pathTemplate{
				pathType: "cross_exchange",
				legs: []Leg{
					{Symbol: symbol, Side: exchange.OrderSideBuy, Exchange: exs[0]},
					{Symbol: symbol, Side: exchange.OrderSideSell, Exchange: exs[1]},
				},
			})
			// Reverse: buy on exchange[1], sell on exchange[0].
			templates = append(templates, pathTemplate{
				pathType: "cross_exchange",
				legs: []Leg{
					{Symbol: symbol, Side: exchange.OrderSideBuy, Exchange: exs[1]},
					{Symbol: symbol, Side: exchange.OrderSideSell, Exchange: exs[0]},
				},
			})
		}
	}
	return templates
}

// EvaluatePath fetches live book tickers and computes whether the path is
// profitable after fees. Returns (path, true, nil) if profitable.
func EvaluatePath(ctx context.Context, tmpl pathTemplate, feeRate float64, minProfitPct float64) (ArbPath, bool, error) {
	tickers := make([]exchange.BookTicker, len(tmpl.legs))
	for i, leg := range tmpl.legs {
		ticker, err := leg.Exchange.GetBookTicker(ctx, leg.Symbol)
		if err != nil {
			return ArbPath{}, false, fmt.Errorf("get book ticker %s: %w", leg.Symbol, err)
		}
		tickers[i] = ticker
	}

	multiplier := computeMultiplier(tmpl.legs, tickers)
	netProfitPct := (multiplier*math.Pow(1-feeRate, float64(len(tmpl.legs))) - 1) * 100

	path := ArbPath{
		Legs:         tmpl.legs,
		Tickers:      tickers,
		Multiplier:   multiplier,
		NetProfitPct: netProfitPct,
		PathType:     tmpl.pathType,
	}

	return path, netProfitPct >= minProfitPct, nil
}

// computeMultiplier returns the gross rate product of a path (before fees).
// Starts with 1.0 USDT and computes the effective multiplier through all legs.
//
//	BUY leg:  multiply by (1 / askPrice)  — you spend quote, receive base
//	SELL leg: multiply by bidPrice        — you spend base, receive quote
//
// The result is dimensioned back to the starting quote asset.
func computeMultiplier(legs []Leg, tickers []exchange.BookTicker) float64 {
	m := 1.0
	for i, leg := range legs {
		ticker := tickers[i]
		switch leg.Side {
		case exchange.OrderSideBuy:
			if ticker.AskPrice == 0 {
				return 0
			}
			m /= ticker.AskPrice
		case exchange.OrderSideSell:
			m *= ticker.BidPrice
		}
	}
	return m
}
