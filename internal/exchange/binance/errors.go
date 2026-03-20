// Package binance wraps the go-binance SDK and implements the exchange.Exchange interface.
package binance

import (
	"errors"
	"fmt"

	"github.com/adshao/go-binance/v2/common"
)

// isBinanceErr checks if an error is a Binance API error with a specific code.
func isBinanceErr(err error, code int) bool {
	var apiErr *common.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == int64(code)
	}
	return false
}

// isOrderAlreadyExists returns true if the error indicates the ClientOrderID
// is already in use on the exchange (code -2010). Treated as a success path.
func isOrderAlreadyExists(err error) bool {
	return isBinanceErr(err, -2010)
}

// wrapAPIError returns a descriptive error string for a Binance API error.
func wrapAPIError(op string, err error) error {
	var apiErr *common.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("%s: binance API error %d: %s: %w", op, apiErr.Code, apiErr.Message, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
