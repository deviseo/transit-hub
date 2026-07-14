package leaderboard

import (
	"encoding/json"
	"testing"
)

func TestEmbedWorkspaceIndexHelpers(t *testing.T) {
	if got := embedSessionKey("token-1"); got != "leaderboard:embed:session:token-1" {
		t.Fatalf("embed session key = %q", got)
	}
	if got := embedWorkspaceIndexKey(" user-1 ", " account-1 "); got != "leaderboard:embed:workspace:user-1:account-1" {
		t.Fatalf("workspace index key = %q", got)
	}
}

func TestEmbedSessionBelongsToWorkspaceRequiresExactMatch(t *testing.T) {
	payload, err := json.Marshal(EmbedSession{UserID: "user-1", AdminAccountID: "account-1"})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if !embedSessionBelongsToWorkspace(string(payload), "user-1", "account-1") {
		t.Fatal("expected exact user/account match")
	}
	if embedSessionBelongsToWorkspace(string(payload), "user-1", "account-2") {
		t.Fatal("must not match another admin account")
	}
	if embedSessionBelongsToWorkspace(string(payload), "user-2", "account-1") {
		t.Fatal("must not match another user")
	}
	if embedSessionBelongsToWorkspace(`{"userId":"user-1"`, "user-1", "account-1") {
		t.Fatal("must not match malformed legacy session payload")
	}
}
