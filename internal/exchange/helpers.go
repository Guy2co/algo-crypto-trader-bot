package exchange

import "context"

// FindBalance searches a slice of balances for the given asset.
// Returns an empty Balance with the asset name set if not found.
func FindBalance(balances []Balance, asset string) Balance {
	for _, b := range balances {
		if b.Asset == asset {
			return b
		}
	}
	return Balance{Asset: asset}
}

// GetBalanceSingle is a convenience wrapper for Exchange implementations whose
// GetBalance can be derived by calling GetBalances and filtering.
func GetBalanceSingle(ctx context.Context, ex Exchange, asset string) (Balance, error) {
	balances, err := ex.GetBalances(ctx)
	if err != nil {
		return Balance{}, err
	}
	return FindBalance(balances, asset), nil
}
