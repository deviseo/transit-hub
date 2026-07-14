package lottery

import (
	"testing"
	"time"
)

func TestRateCleanupAtForRewardSchedulesSuccessfulDedicatedRate(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	job := RewardJob{Prize: Prize{Type: PrizeTypeSubscription, Multiplier: "0.9", ValidityDays: intPtr(30)}}
	cleanupAt := rateCleanupAtForReward(job, RewardResult{Status: RewardFulfilled}, now)
	if cleanupAt == nil || !cleanupAt.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("cleanupAt = %v", cleanupAt)
	}
}

func TestRateCleanupAtForRewardSkipsLegacyAndMissingUserRewards(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	validityDays := intPtr(30)
	tests := []struct {
		name   string
		job    RewardJob
		result RewardResult
	}{
		{name: "legacy subscription", job: RewardJob{Prize: Prize{Type: PrizeTypeSubscription, ValidityDays: validityDays}}, result: RewardResult{Status: RewardFulfilled}},
		{name: "missing upstream user", job: RewardJob{Prize: Prize{Type: PrizeTypeSubscription, Multiplier: "0.9", ValidityDays: validityDays}}, result: RewardResult{Status: RewardFulfilled, SkipRateCleanup: true}},
		{name: "failed apply", job: RewardJob{Prize: Prize{Type: PrizeTypeSubscription, Multiplier: "0.9", ValidityDays: validityDays}}, result: RewardResult{Status: RewardRetryableFailed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cleanupAt := rateCleanupAtForReward(tt.job, tt.result, now); cleanupAt != nil {
				t.Fatalf("cleanupAt = %v, want nil", cleanupAt)
			}
		})
	}
}
