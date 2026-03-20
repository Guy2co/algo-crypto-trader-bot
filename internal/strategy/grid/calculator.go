package grid

import "math"

// ComputeLevels returns count+1 evenly-spaced price levels between bottom and top
// (inclusive), forming count intervals.
func ComputeLevels(bottom, top float64, count int) []float64 {
	if count < 1 {
		return nil
	}
	spacing := (top - bottom) / float64(count)
	levels := make([]float64, count+1)
	for i := range levels {
		levels[i] = bottom + float64(i)*spacing
	}
	return levels
}

// ComputeQuantityPerGrid returns the base asset quantity allocated to each
// grid interval given a total USDT investment and the mid-price.
//
//	qty = (investment / count) / midPrice
func ComputeQuantityPerGrid(investment float64, count int, midPrice float64) float64 {
	if count == 0 || midPrice == 0 {
		return 0
	}
	return (investment / float64(count)) / midPrice
}

// ComputeTheoreticalProfit returns the expected profit per completed grid cycle.
//
//	profit = (gridSpacing / avgPrice) - (2 * feeRate)
func ComputeTheoreticalProfit(gridSpacing, avgPrice, feeRate float64) float64 {
	if avgPrice == 0 {
		return 0
	}
	return gridSpacing/avgPrice - 2*feeRate
}

// RoundToTickSize rounds a price down to the nearest valid exchange tick size.
func RoundToTickSize(price, tickSize float64) float64 {
	if tickSize == 0 {
		return price
	}
	return math.Floor(price/tickSize) * tickSize
}

// RoundToStepSize rounds a quantity down to the nearest valid exchange lot step size.
func RoundToStepSize(qty, stepSize float64) float64 {
	if stepSize == 0 {
		return qty
	}
	return math.Floor(qty/stepSize) * stepSize
}
