package lottery

import (
	"errors"
	"testing"
)

func TestBuildDraftValidatesCampaignTiming(t *testing.T) {
	service := &Service{}
	cases := []struct {
		name string
		req  CreateCampaignRequest
	}{
		{
			name: "registration end before start",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.RegistrationEnd = "2026-07-13T09:00:00Z" }),
		},
		{
			name: "scheduled draw requires draw time",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.DrawMode = DrawModeScheduled; req.DrawAt = "" }),
		},
		{
			name: "draw before registration end",
			req: validCampaignRequest(func(req *CreateCampaignRequest) {
				req.DrawMode = DrawModeScheduled
				req.DrawAt = "2026-07-13T11:00:00Z"
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := service.buildDraft("user-1", "acct-1", "campaign-1", tc.req)
			if !errors.Is(err, requestError(ErrorValidation)) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestBuildDraftValidatesPrizeFieldsBeforeDatabaseCasts(t *testing.T) {
	service := &Service{}
	cases := []struct {
		name string
		req  CreateCampaignRequest
	}{
		{
			name: "invalid balance amount",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.Prizes[0].BalanceAmount = "abc" }),
		},
		{
			name: "nonpositive balance amount",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.Prizes[0].BalanceAmount = "0" }),
		},
		{
			name: "invalid multiplier",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.Prizes[0].Multiplier = "NaN" }),
		},
		{
			name: "balance prize cannot include multiplier",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.Prizes[0].Multiplier = "1.5" }),
		},
		{
			name: "balance prize cannot include subscription group",
			req:  validCampaignRequest(func(req *CreateCampaignRequest) { req.Prizes[0].GroupID = "group-1" }),
		},
		{
			name: "subscription requires group",
			req: validCampaignRequest(func(req *CreateCampaignRequest) {
				req.Prizes[0] = PrizeRequest{Type: PrizeTypeSubscription, Name: "Subscription", Quantity: 1, ValidityDays: intPointer(30)}
			}),
		},
		{
			name: "subscription rejects balance amount",
			req: validCampaignRequest(func(req *CreateCampaignRequest) {
				req.Prizes[0] = PrizeRequest{Type: PrizeTypeSubscription, Name: "Subscription", Quantity: 1, GroupID: "group-1", ValidityDays: intPointer(30), BalanceAmount: "1"}
			}),
		},
		{
			name: "subscription validates validity days",
			req: validCampaignRequest(func(req *CreateCampaignRequest) {
				req.Prizes[0] = PrizeRequest{Type: PrizeTypeSubscription, Name: "Subscription", Quantity: 1, GroupID: "group-1", ValidityDays: intPointer(36501)}
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := service.buildDraft("user-1", "acct-1", "campaign-1", tc.req)
			if !errors.Is(err, requestError(ErrorValidation)) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestBuildDraftAcceptsValidBalanceAndSubscriptionPrizes(t *testing.T) {
	service := &Service{}
	req := validCampaignRequest(func(req *CreateCampaignRequest) {
		req.DrawMode = DrawModeScheduled
		req.Prizes = append(req.Prizes, PrizeRequest{Type: PrizeTypeSubscription, Name: "Subscription", Quantity: 1, GroupID: "group-1", Multiplier: "1.5", ValidityDays: intPointer(30)})
	})
	campaign, prizes, err := service.buildDraft("user-1", "acct-1", "campaign-1", req)
	if err != nil {
		t.Fatalf("buildDraft returned err=%v", err)
	}
	if campaign.DrawMode != DrawModeScheduled || len(prizes) != 2 {
		t.Fatalf("unexpected campaign=%#v prizes=%#v", campaign, prizes)
	}
}

func TestBuildDraftValidatesBalancePrizeDelivery(t *testing.T) {
	service := &Service{}

	validVoucher := validCampaignRequest(func(req *CreateCampaignRequest) {
		req.Prizes[0].Quantity = 2
		req.Prizes[0].DeliveryMode = DeliveryVoucher
		req.Prizes[0].VoucherCodes = []string{" code-a ", "code-b"}
	})
	_, prizes, err := service.buildDraft("user-1", "acct-1", "campaign-1", validVoucher)
	if err != nil {
		t.Fatalf("valid voucher delivery returned err=%v", err)
	}
	if got := prizes[0].VoucherCodes[0]; got != "code-a" {
		t.Fatalf("voucher code was not normalized: %q", got)
	}

	invalidCases := []CreateCampaignRequest{
		validCampaignRequest(func(req *CreateCampaignRequest) {
			req.Prizes[0].Quantity = 2
			req.Prizes[0].DeliveryMode = DeliveryVoucher
			req.Prizes[0].VoucherCodes = []string{"only-one"}
		}),
		validCampaignRequest(func(req *CreateCampaignRequest) {
			req.Prizes[0].Quantity = 2
			req.Prizes[0].DeliveryMode = DeliveryVoucher
			req.Prizes[0].VoucherCodes = []string{"same", "same"}
		}),
		validCampaignRequest(func(req *CreateCampaignRequest) {
			req.Prizes[0].DeliveryMode = DeliveryManual
		}),
	}
	for i, req := range invalidCases {
		if _, _, err := service.buildDraft("user-1", "acct-1", "campaign-1", req); !errors.Is(err, requestError(ErrorValidation)) {
			t.Fatalf("case %d expected validation error, got %v", i, err)
		}
	}
}

func TestBuildDraftAcceptsManualBalanceDelivery(t *testing.T) {
	service := &Service{}
	req := validCampaignRequest(func(req *CreateCampaignRequest) {
		req.Prizes[0].DeliveryMode = DeliveryManual
		req.Prizes[0].ManualContact = " support@example.com "
	})
	_, prizes, err := service.buildDraft("user-1", "acct-1", "campaign-1", req)
	if err != nil {
		t.Fatalf("valid manual delivery returned err=%v", err)
	}
	if prizes[0].ManualContact != "support@example.com" {
		t.Fatalf("manual contact was not normalized: %q", prizes[0].ManualContact)
	}
}

func validCampaignRequest(mutators ...func(*CreateCampaignRequest)) CreateCampaignRequest {
	req := CreateCampaignRequest{
		Name:              "Campaign",
		RegistrationStart: "2026-07-13T10:00:00Z",
		RegistrationEnd:   "2026-07-13T12:00:00Z",
		DrawAt:            "2026-07-13T13:00:00Z",
		DrawMode:          DrawModeManual,
		Prizes: []PrizeRequest{
			{Type: PrizeTypeBalance, Name: "Balance", Quantity: 1, BalanceAmount: "12.50"},
		},
	}
	for _, mutate := range mutators {
		mutate(&req)
	}
	return req
}

func intPointer(value int) *int { return &value }
