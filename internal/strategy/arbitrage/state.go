package arbitrage

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/Guy2co/algo-crypto-trader-bot/internal/state"
)

// ArbMetrics tracks running performance of the arbitrage strategy.
type ArbMetrics struct {
	TotalCycles        int       `json:"total_cycles"`
	TotalProfit        float64   `json:"total_profit_usdt"`
	TotalFeesPaid      float64   `json:"total_fees_paid"`
	TotalOpportunities int       `json:"total_opportunities_seen"`
	TotalExecuted      int       `json:"total_executed"`
	StartTime          time.Time `json:"start_time"`
	LastCycleTime      time.Time `json:"last_cycle_time"`
}

// ArbState is the persisted state of the arbitrage strategy.
type ArbState struct {
	mu      sync.RWMutex `json:"-"`
	RunID   string       `json:"run_id"`
	Metrics ArbMetrics   `json:"metrics"`
}

func arbStatePath(stateDir string) string {
	return filepath.Join(stateDir, "triangular-arb.json")
}

func (s *ArbState) save(stateDir string) error {
	s.mu.RLock()
	err := state.SaveJSON(arbStatePath(stateDir), s)
	s.mu.RUnlock()
	return err
}

func loadArbState(stateDir string) (*ArbState, error) {
	var s ArbState
	found, err := state.LoadJSON(arbStatePath(stateDir), &s)
	if err != nil || !found {
		return nil, err
	}
	return &s, nil
}
