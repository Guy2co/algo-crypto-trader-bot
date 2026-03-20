package grid

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/state"
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
	err := state.SaveJSON(statePath(stateDir, s.Symbol), s)
	s.mu.RUnlock()
	return err
}

// loadState loads GridState from disk. Returns (nil, nil) if no file exists.
func loadState(stateDir, symbol string) (*GridState, error) {
	var s GridState
	found, err := state.LoadJSON(statePath(stateDir, symbol), &s)
	if err != nil || !found {
		return nil, err
	}
	return &s, nil
}
