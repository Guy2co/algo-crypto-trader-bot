package exchange

import (
	"math"
	"strconv"
)

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

// CountDecimals returns the number of decimal places in a numeric string
// such as "0.00100000" → 8.
func CountDecimals(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return len(s) - i - 1
		}
	}
	return 0
}

// ParsePrice parses a price string to float64, returning 0 on error.
// Convenience wrapper for exchange implementations that receive string prices from APIs.
func ParsePrice(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
