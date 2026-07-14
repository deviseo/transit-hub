package lottery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRewardJobStaleCutoffUsesTypedTimestamp(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	got := rewardJobStaleCutoff(15*time.Minute, now)
	want := now.Add(-15 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", got, want)
	}
}

func TestRewardJobStaleCutoffDefaultsNonPositiveDuration(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	got := rewardJobStaleCutoff(0, now)
	want := now.Add(-10 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("cutoff = %s, want %s", got, want)
	}
}

func TestCompleteClaimedRewardJobDataRejectsMissingRelations(t *testing.T) {
	job := RewardJob{ID: "job-1", WinnerID: "winner-1", PrizeID: "prize-1"}
	if _, err := completeClaimedRewardJobData(job, nil, &Prize{ID: "prize-1"}); err == nil || !strings.Contains(err.Error(), "missing winner") {
		t.Fatalf("expected missing winner error, got %v", err)
	}
	if _, err := completeClaimedRewardJobData(job, &Winner{ID: "winner-1"}, nil); err == nil || !strings.Contains(err.Error(), "missing prize") {
		t.Fatalf("expected missing prize error, got %v", err)
	}
}

func TestCompleteClaimedRewardJobDataAttachesRelations(t *testing.T) {
	job, err := completeClaimedRewardJobData(RewardJob{ID: "job-1", WinnerID: "winner-1", PrizeID: "prize-1"}, &Winner{ID: "winner-1", Sub2apiUserID: "viewer-1"}, &Prize{ID: "prize-1", Type: PrizeTypeBalance, BalanceAmount: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Winner.ID != "winner-1" || job.Winner.Sub2apiUserID != "viewer-1" || job.Prize.ID != "prize-1" || job.Prize.Type != PrizeTypeBalance {
		t.Fatalf("relations were not attached: %+v", job)
	}
}

func TestRewardDeliveryForSlot(t *testing.T) {
	status, errorKey, reference, err := rewardDeliveryForSlot(Prize{DeliveryMode: DeliveryVoucher, VoucherCodes: []string{"voucher-a", "voucher-b"}}, 1)
	if err != nil || status != RewardFulfilled || errorKey != "" || reference != "voucher-b" {
		t.Fatalf("unexpected voucher delivery: status=%q errorKey=%q reference=%q err=%v", status, errorKey, reference, err)
	}

	status, errorKey, reference, err = rewardDeliveryForSlot(Prize{DeliveryMode: DeliveryManual}, 0)
	if err != nil || status != RewardManualAttention || errorKey != ErrorRewardManualRequired || reference != "" {
		t.Fatalf("unexpected manual delivery: status=%q errorKey=%q reference=%q err=%v", status, errorKey, reference, err)
	}

	if _, _, _, err := rewardDeliveryForSlot(Prize{DeliveryMode: DeliveryVoucher, VoucherCodes: []string{"voucher-a"}}, 1); !errors.Is(err, requestError(ErrorValidation)) {
		t.Fatalf("expected missing voucher validation error, got %v", err)
	}
}
