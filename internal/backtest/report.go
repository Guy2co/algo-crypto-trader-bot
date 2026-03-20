package backtest

import (
	"fmt"
	"math"
	"time"
)

// EquityPoint records portfolio value at a point in time.
type EquityPoint struct {
	Time   time.Time
	Equity float64
}

// Report holds the result of a completed backtest simulation.
type Report struct {
	Symbol           string
	StartDate        time.Time
	EndDate          time.Time
	InitialUSDT      float64
	FinalUSDT        float64
	TotalReturnPct   float64
	AnnualizedReturn float64
	MaxDrawdownPct   float64
	SharpeRatio      float64
	TotalTrades      int
	CompletedCycles  int
	TotalFeesPaid    float64
	EquityCurve      []EquityPoint
}

// Print outputs the backtest report as a formatted text table.
func (r *Report) Print() {
	fmt.Println("============================================================")
	fmt.Printf("  Backtest Report — %s\n", r.Symbol)
	fmt.Println("============================================================")
	fmt.Printf("  Period       : %s → %s\n", r.StartDate.Format("2006-01-02"), r.EndDate.Format("2006-01-02"))
	fmt.Printf("  Initial USDT : %.2f\n", r.InitialUSDT)
	fmt.Printf("  Final USDT   : %.2f\n", r.FinalUSDT)
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("  Total Return : %.2f%%\n", r.TotalReturnPct)
	fmt.Printf("  Ann. Return  : %.2f%%\n", r.AnnualizedReturn)
	fmt.Printf("  Max Drawdown : %.2f%%\n", r.MaxDrawdownPct)
	fmt.Printf("  Sharpe Ratio : %.2f\n", r.SharpeRatio)
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("  Total Trades : %d\n", r.TotalTrades)
	fmt.Printf("  Cycles Done  : %d\n", r.CompletedCycles)
	fmt.Printf("  Fees Paid    : %.4f USDT\n", r.TotalFeesPaid)
	fmt.Println("============================================================")
}

// computeReport builds the Report from an equity curve.
func computeReport(symbol string, initial float64, curve []EquityPoint, totalTrades, cycles int, feesPaid float64) *Report {
	if len(curve) == 0 {
		return &Report{Symbol: symbol, InitialUSDT: initial}
	}

	final := curve[len(curve)-1].Equity
	totalRet := (final - initial) / initial * 100

	// Annualized return (CAGR).
	days := curve[len(curve)-1].Time.Sub(curve[0].Time).Hours() / 24
	var annRet float64
	if days > 0 {
		years := days / 365
		annRet = (math.Pow(final/initial, 1/years) - 1) * 100
	}

	// Max drawdown.
	maxDD := computeMaxDrawdown(curve)

	// Sharpe ratio (daily returns, risk-free = 0).
	sharpe := computeSharpe(curve)

	return &Report{
		Symbol:           symbol,
		StartDate:        curve[0].Time,
		EndDate:          curve[len(curve)-1].Time,
		InitialUSDT:      initial,
		FinalUSDT:        final,
		TotalReturnPct:   totalRet,
		AnnualizedReturn: annRet,
		MaxDrawdownPct:   maxDD,
		SharpeRatio:      sharpe,
		TotalTrades:      totalTrades,
		CompletedCycles:  cycles,
		TotalFeesPaid:    feesPaid,
		EquityCurve:      curve,
	}
}

func computeMaxDrawdown(curve []EquityPoint) float64 {
	peak := curve[0].Equity
	maxDD := 0.0
	for _, p := range curve {
		if p.Equity > peak {
			peak = p.Equity
		}
		dd := (peak - p.Equity) / peak * 100
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

func computeSharpe(curve []EquityPoint) float64 {
	if len(curve) < 2 {
		return 0
	}
	returns := make([]float64, 0, len(curve)-1)
	for i := 1; i < len(curve); i++ {
		if curve[i-1].Equity == 0 {
			continue
		}
		ret := (curve[i].Equity - curve[i-1].Equity) / curve[i-1].Equity
		returns = append(returns, ret)
	}
	if len(returns) == 0 {
		return 0
	}

	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	var variance float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(returns))
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		return 0
	}
	return mean / stddev * math.Sqrt(365)
}
