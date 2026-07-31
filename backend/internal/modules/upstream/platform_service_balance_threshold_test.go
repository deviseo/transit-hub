package upstream

import (
	"math"
	"testing"
)

func TestSumFilteredBalances_Sub2APIThreshold(t *testing.T) {
	users := []any{
		map[string]any{"balance": 4999.99},
		map[string]any{"balance": 5000.0},
		map[string]any{"balance": 5000.01},
		map[string]any{"role": "admin", "balance": 4999.99},
		map[string]any{"balance": math.NaN()},
		map[string]any{"balance": math.Inf(1)},
		map[string]any{"balance": math.Inf(-1)},
		map[string]any{"email": "missing-balance"},
		map[string]any{"balance": "not-a-number"},
	}

	tests := []struct {
		name   string
		filter BalanceFilter
		want   float64
	}{
		{
			name:   "no threshold keeps every finite non-admin balance",
			filter: BalanceFilter{ExcludeAdmin: true},
			want:   15000,
		},
		{
			name:   "threshold excludes only balances strictly above it",
			filter: BalanceFilter{ExcludeBalances: []float64{5000}},
			want:   14999.98,
		},
		{
			name:   "admin exclusion combines with threshold",
			filter: BalanceFilter{ExcludeAdmin: true, ExcludeBalances: []float64{5000}},
			want:   9999.99,
		},
		{
			name:   "minimum finite non-negative value is the threshold",
			filter: BalanceFilter{ExcludeBalances: []float64{math.NaN(), math.Inf(1), -1, 6000, 5000, math.Inf(-1)}},
			want:   14999.98,
		},
		{
			name:   "invalid threshold configuration does not enable filtering",
			filter: BalanceFilter{ExcludeBalances: []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1}},
			want:   19999.99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sumFilteredBalances(users, tt.filter)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("sumFilteredBalances() = %.12f, want %.12f", got, tt.want)
			}
		})
	}
}

func TestSumNewAPIFilteredQuotas_ExactBalanceUnchanged(t *testing.T) {
	users := []any{
		map[string]any{"role": 1, "quota": 5000.0},
		map[string]any{"role": 1, "quota": 5001.0},
	}

	got := sumNewAPIFilteredQuotas(users, BalanceFilter{ExcludeBalances: []float64{5000}}, 1)
	if got != 5001 {
		t.Fatalf("sumNewAPIFilteredQuotas() = %.12f, want 5001", got)
	}
}
