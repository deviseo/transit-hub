package admin_accounts

import (
	"strings"
	"testing"
)

func TestWorkspaceDeleteStatementsCoverAllWorkspaceTables(t *testing.T) {
	want := []string{
		"lottery_reward_jobs",
		"lottery_winners",
		"lottery_draws",
		"lottery_entries",
		"lottery_prizes",
		"lottery_audit_logs",
		"lottery_campaigns",
		"lottery_embed_configs",
		"mass_email_batch_items",
		"mass_email_batches",
		"group_rate_campaign_items",
		"group_rate_campaigns",
		"connection_health_target_action_states",
		"connection_health_probe_budget_usage",
		"connection_health_priority_sync_states",
		"connection_health_group_target_exclusions",
		"connection_health_group_policy_assignments",
		"connection_health_policy_assignments",
		"connection_health_model_targets",
		"connection_health_states",
		"connection_health_events",
		"connection_health_policies",
		"ticket_attachments",
		"ticket_messages",
		"tickets",
		"ticket_embed_configs",
		"leaderboard_embed_configs",
		"group_rate_snapshots",
		"strategy_settings",
		"notification_channel_settings",
		"smtp_settings",
		"email_templates",
		"my_site_states",
		"real_connections",
		"dashboard_daily_stats",
		"dashboard_balance_filter",
		"upstream_sites",
	}
	if len(workspaceDeleteStatements) != len(want) {
		t.Fatalf("delete statement count = %d, want %d", len(workspaceDeleteStatements), len(want))
	}

	seen := map[string]string{}
	for i, stmt := range workspaceDeleteStatements {
		if stmt.Name != want[i] {
			t.Fatalf("delete statement order[%d] = %s, want %s", i, stmt.Name, want[i])
		}
		seen[stmt.Name] = stmt.SQL
	}
	for _, table := range want {
		sql, ok := seen[table]
		if !ok {
			t.Fatalf("missing delete statement for %s", table)
		}
		if !strings.Contains(sql, "user_id = $1") {
			t.Fatalf("%s delete is not scoped by user_id: %s", table, sql)
		}
		workspaceColumn := "admin_account_id = $2"
		if table == "real_connections" {
			workspaceColumn = "workspace_admin_account_id = $2"
		}
		if !strings.Contains(sql, workspaceColumn) {
			t.Fatalf("%s delete is not scoped by workspace column %q: %s", table, workspaceColumn, sql)
		}
	}
}

func TestLegacyWorkspaceDescriptorsCoverDeleteTables(t *testing.T) {
	descriptors := map[string]string{}
	for _, table := range legacyWorkspaceTables {
		descriptors[table.Name] = table.WorkspaceColumn
	}
	for _, stmt := range workspaceDeleteStatements {
		column, ok := descriptors[stmt.Name]
		if !ok {
			t.Fatalf("legacy workspace descriptor missing for delete table %s", stmt.Name)
		}
		wantColumn := "admin_account_id"
		if stmt.Name == "real_connections" {
			wantColumn = "workspace_admin_account_id"
		}
		if column != wantColumn {
			t.Fatalf("legacy descriptor column for %s = %s, want %s", stmt.Name, column, wantColumn)
		}
	}
}

func TestWorkspaceDeleteStatementsDeleteChildrenBeforeParents(t *testing.T) {
	order := map[string]int{}
	for i, stmt := range workspaceDeleteStatements {
		order[stmt.Name] = i
	}
	assertBefore := func(child string, parent string) {
		t.Helper()
		if order[child] >= order[parent] {
			t.Fatalf("%s must be deleted before %s", child, parent)
		}
	}

	assertBefore("ticket_attachments", "ticket_messages")
	assertBefore("ticket_messages", "tickets")
	assertBefore("mass_email_batch_items", "mass_email_batches")
	assertBefore("group_rate_campaign_items", "group_rate_campaigns")
	assertBefore("connection_health_group_policy_assignments", "connection_health_policies")
	assertBefore("connection_health_policy_assignments", "connection_health_policies")
	assertBefore("connection_health_model_targets", "connection_health_policies")
}

func TestWorkspaceDeleteSQLDocumentsCurrentFallbackAndLocks(t *testing.T) {
	if !strings.Contains(nextCurrentWorkspaceIDSQL, "ORDER BY updated_at DESC, id ASC") {
		t.Fatal("current fallback must be deterministic")
	}
	if !strings.Contains(lockUserForWorkspaceDeleteSQL, "FOR UPDATE") {
		t.Fatal("user row must be locked")
	}
	if !strings.Contains(lockAccountForWorkspaceDeleteSQL, "user_id = $1") || !strings.Contains(lockAccountForWorkspaceDeleteSQL, "id = $2") || !strings.Contains(lockAccountForWorkspaceDeleteSQL, "FOR UPDATE") {
		t.Fatal("account row must be locked under user/account scope")
	}
}
