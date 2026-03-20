package arbitrage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal arb state: %w", err)
	}
	path := arbStatePath(stateDir)
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write arb state to %s: %w", path, err)
	}
	return nil
}

func loadArbState(stateDir string) (*ArbState, error) {
	path := arbStatePath(stateDir)
	data, err := os.ReadFile(path) //nolint:gosec
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read arb state from %s: %w", path, err)
	}
	var s ArbState
	if err = json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal arb state: %w", err)
	}
	return &s, nil
}
