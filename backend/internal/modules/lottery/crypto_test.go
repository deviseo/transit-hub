package lottery

import "testing"

func TestDeterministicShuffleIsReproducibleAndOnePrizePerUser(t *testing.T) {
	entries := []Entry{
		{ID: "entry-1", Sub2apiUserID: "user-1", ReceiptHash: "r1"},
		{ID: "entry-2", Sub2apiUserID: "user-2", ReceiptHash: "r2"},
		{ID: "entry-3", Sub2apiUserID: "user-3", ReceiptHash: "r3"},
	}
	hash, err := snapshotHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	seed := finalSeed("secret", "campaign-1", hash)
	first, err := deterministicShuffle(entries, seed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := deterministicShuffle(entries, seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(entries) || len(second) != len(entries) {
		t.Fatalf("shuffle length mismatch")
	}
	seen := map[string]bool{}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("shuffle not reproducible at %d", i)
		}
		if seen[first[i].Sub2apiUserID] {
			t.Fatalf("duplicate winner user %s", first[i].Sub2apiUserID)
		}
		seen[first[i].Sub2apiUserID] = true
	}
}

func TestExpandedPrizeSlotsUsesSortOrderAndHandlesInsufficientEntrants(t *testing.T) {
	prizes := []Prize{{ID: "p2", SortOrder: 20, Quantity: 2}, {ID: "p1", SortOrder: 10, Quantity: 1}}
	slots := expandedPrizeSlots(prizes)
	if len(slots) != 3 || slots[0].ID != "p1" || slots[1].ID != "p2" || slots[2].ID != "p2" {
		t.Fatalf("unexpected slots: %#v", slots)
	}
	entries := []Entry{{ID: "e1"}, {ID: "e2"}}
	if len(slots) > len(entries) {
		slots = slots[:len(entries)]
	}
	if len(slots) != 2 {
		t.Fatalf("slots after entrant cap = %d, want 2", len(slots))
	}
}

func TestCommitmentSnapshotMaskAndViewerStatus(t *testing.T) {
	first := seedCommitment("secret")
	second := seedCommitment("secret")
	if first == "secret" || first != second {
		t.Fatal("commitment must be deterministic and not reveal secret")
	}
	if maskEmail("alice@example.com") != "a***@example.com" {
		t.Fatalf("unexpected masked email")
	}
	if !viewerActive("") || !viewerActive("active") || viewerActive("disabled") {
		t.Fatalf("viewer active status classification failed")
	}
}

func TestPublicV2SnapshotCanBeRebuiltWithoutPrivateUserID(t *testing.T) {
	entries := []Entry{{ID: "entry-1", Sub2apiUserID: "private-user-a", MaskedEmail: "a***@example.com", ReceiptHash: "receipt-a"}}
	first, err := snapshotHashForVersion(entries, AlgorithmVersionV2)
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Sub2apiUserID = "private-user-b"
	second, err := snapshotHashForVersion(entries, AlgorithmVersionV2)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("public v2 snapshot changed with private user id: %s != %s", first, second)
	}
	entries[0].ReceiptHash = "receipt-b"
	third, err := snapshotHashForVersion(entries, AlgorithmVersionV2)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatalf("public v2 snapshot ignored public receipt hash: %s", first)
	}
}
