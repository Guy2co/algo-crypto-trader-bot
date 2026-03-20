package grid

import (
	"math"
	"testing"
)

func TestComputeLevels(t *testing.T) {
	t.Parallel()

	levels := ComputeLevels(100, 200, 10)

	if len(levels) != 11 {
		t.Fatalf("expected 11 levels, got %d", len(levels))
	}
	if levels[0] != 100 {
		t.Errorf("expected bottom 100, got %f", levels[0])
	}
	if levels[10] != 200 {
		t.Errorf("expected top 200, got %f", levels[10])
	}

	// Spacing should be uniform.
	spacing := levels[1] - levels[0]
	for i := 1; i < len(levels); i++ {
		got := levels[i] - levels[i-1]
		if math.Abs(got-spacing) > 1e-9 {
			t.Errorf("level %d: spacing %.9f, want %.9f", i, got, spacing)
		}
	}
}

func TestComputeLevels_ZeroCount(t *testing.T) {
	t.Parallel()
	levels := ComputeLevels(100, 200, 0)
	if levels != nil {
		t.Errorf("expected nil for zero count, got %v", levels)
	}
}

func TestComputeQuantityPerGrid(t *testing.T) {
	t.Parallel()

	// 1000 USDT / 10 grids / 50000 price = 0.002 BTC per grid
	qty := ComputeQuantityPerGrid(1000, 10, 50000)
	want := 0.002
	if math.Abs(qty-want) > 1e-10 {
		t.Errorf("got %.10f, want %.10f", qty, want)
	}
}

func TestComputeQuantityPerGrid_ZeroInputs(t *testing.T) {
	t.Parallel()
	if got := ComputeQuantityPerGrid(1000, 0, 50000); got != 0 {
		t.Errorf("zero count should return 0, got %f", got)
	}
	if got := ComputeQuantityPerGrid(1000, 10, 0); got != 0 {
		t.Errorf("zero price should return 0, got %f", got)
	}
}

func TestComputeTheoreticalProfit(t *testing.T) {
	t.Parallel()

	// spacing=100, avgPrice=90000, feeRate=0.001
	profit := ComputeTheoreticalProfit(100, 90000, 0.001)
	// Expected: 100/90000 - 2*0.001 = 0.001111 - 0.002 = negative (tight grid)
	want := 100.0/90000.0 - 2*0.001
	if math.Abs(profit-want) > 1e-10 {
		t.Errorf("got %.10f, want %.10f", profit, want)
	}
}

func TestRoundToTickSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		price    float64
		tickSize float64
		want     float64
	}{
		{90001.5, 0.01, 90001.5},
		{90001.555, 0.01, 90001.55},
		{90001.555, 1, 90001},
		{100, 0, 100}, // zero tick → return as-is
	}

	for _, tt := range tests {
		got := RoundToTickSize(tt.price, tt.tickSize)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("RoundToTickSize(%.4f, %.4f) = %.4f, want %.4f", tt.price, tt.tickSize, got, tt.want)
		}
	}
}

func TestRoundToStepSize(t *testing.T) {
	t.Parallel()

	got := RoundToStepSize(0.123456789, 0.00001)
	want := 0.12345
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %.8f, want %.8f", got, want)
	}
}
