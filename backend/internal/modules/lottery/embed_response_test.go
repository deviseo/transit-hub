package lottery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCampaignResponseOmitsViewerFieldsWhenAbsent(t *testing.T) {
	payload, err := json.Marshal(CampaignResponse{ID: "campaign-1", Name: "Campaign", DrawMode: DrawModeManual})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{"myEntry", "myWinner", "myRewardStatus"} {
		if strings.Contains(encoded, field) {
			t.Fatalf("expected %s to be omitted from %s", field, encoded)
		}
	}
}

func TestCampaignResponseIncludesEmbedViewerStateWithoutPrivateFields(t *testing.T) {
	response := CampaignResponse{
		ID:       "campaign-1",
		Name:     "Campaign",
		DrawMode: DrawModeManual,
		MyEntry: &EntryResponse{
			ID:          "entry-1",
			CampaignID:  "campaign-1",
			MaskedEmail: "a***@example.com",
			ReceiptHash: "receipt-hash",
			Status:      EntryStatusWithdrawn,
		},
		MyWinner: &WinnerResponse{
			ID:          "winner-1",
			PrizeID:     "prize-1",
			EntryID:     "entry-1",
			MaskedEmail: "a***@example.com",
			PrizeSlot:   1,
		},
		MyRewardStatus: &MyRewardStatus{
			ID:       "job-1",
			WinnerID: "winner-1",
			PrizeID:  "prize-1",
			Status:   RewardRetryableFailed,
			ErrorKey: ErrorRewardAdminSession,
		},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{"myEntry", "myWinner", "myRewardStatus"} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("expected %s in %s", field, encoded)
		}
	}
	for _, private := range []string{"errorDetail", "remoteRef", "remote_reference", "adminAccountId", "sub2apiUserId", "receiptToken", "seedSecret"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("private field %s leaked in %s", private, encoded)
		}
	}
	if !strings.Contains(encoded, `"status":"withdrawn"`) {
		t.Fatalf("withdrawn entry status was not preserved in %s", encoded)
	}
}

func TestRedactPrizeDeliverySecretsPreservesPublicDeliveryMode(t *testing.T) {
	prizes := redactPrizeDeliverySecrets([]Prize{{
		ID:            "prize-1",
		DeliveryMode:  DeliveryVoucher,
		ManualContact: "private-contact",
		VoucherCodes:  []string{"secret-code"},
	}})
	if prizes[0].DeliveryMode != DeliveryVoucher {
		t.Fatalf("delivery mode should remain public: %+v", prizes[0])
	}
	if prizes[0].ManualContact != "" || len(prizes[0].VoucherCodes) != 0 {
		t.Fatalf("delivery secrets leaked after redaction: %+v", prizes[0])
	}
}
