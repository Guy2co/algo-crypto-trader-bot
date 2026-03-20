package grid

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GridLevel represents one price level in the grid.
type GridLevel struct {
	Index         int     `json:"index"`
	Price         float64 `json:"price"`
	BuyOrderID    int64   `json:"buy_order_id"`
	SellOrderID   int64   `json:"sell_order_id"`
	BuyClientOID  string  `json:"buy_client_oid"`
	SellClientOID string  `json:"sell_client_oid"`
}

// GridMetrics tracks running performance of the strategy.
type GridMetrics struct {
	TotalCycles     int       `json:"total_cycles"`
	TotalProfit     float64   `json:"total_profit_usdt"`
	TotalFeesPaid   float64   `json:"total_fees_paid"`
	StartTime       time.Time `json:"start_time"`
	LastCycleTime   time.Time `json:"last_cycle_time"`
}

// GridState is the full persisted state of a running grid strategy.
type GridState struct {
	mu sync.RWMutex `json:"-"`

	RunID           string      `json:"run_id"`
	Symbol          string      `json:"symbol"`
	GridBottom      float64     `json:"grid_bottom"`
	GridTop         float64     `json:"grid_top"`
	GridCount       int         `json:"grid_count"`
	GridSpacing     float64     `json:"grid_spacing"`
	Investment      float64     `json:"investment"`
	QuantityPerGrid float64     `json:"quantity_per_grid"`
	InitialPrice    float64     `json:"initial_price"`
	Levels          []GridLevel `json:"levels"`
	Metrics         GridMetrics `json:"metrics"`
}

// statePath returns the file path for persisting grid state.
func statePath(stateDir, symbol string) string {
	return filepath.Join(stateDir, symbol+"-grid.json")
}

// save persists the GridState to disk as JSON. Must not be called with mu held.
func (s *GridState) save(stateDir string) error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal grid state: %w", err)
	}

	path := statePath(stateDir, s.Symbol)
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write grid state to %s: %w", path, err)
	}
	return nil
}

// loadState loads GridState from disk. Returns (nil, nil) if no file exists.
func loadState(stateDir, symbol string) (*GridState, error) {
	path := statePath(stateDir, symbol)
	data, err := os.ReadFile(path) //nolint:gosec
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read grid state from %s: %w", path, err)
	}

	var s GridState
	if err = json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal grid state: %w", err)
	}
	return &s, nil
}
