package admin_accounts

import (
	"strings"
	"testing"
)

func TestWorkspaceDeleteStatementsCoverAllWorkspaceTables(t *testing.T) {
	want := []string{
		"mass_email_batch_items",
		"mass_email_batches",
		"group_rate_snapshots",
		"user_activity_daily",
		"user_activity_recharge_daily",
		"user_activity_site_daily",
		"user_activity_profiles",
		"user_activity_sync_states",
		"email_suppressions",
		"profit_route_daily",
		"profit_upstream_daily",
		"profit_routes",
		"profit_sync_states",
		"profit_price_changes",
		"strategy_settings",
		"notification_channel_settings",
		"smtp_settings",
		"email_templates",
		"my_site_states",
		"dashboard_daily_stats",
		"client_configs",
		"dashboard_balance_filter",
		"dashboard_pricing",
		"upstream_sites",
	}
	assertWorkspaceDeleteStatements(t, workspaceDeleteStatements, want)
}

// 已下线功能的表：删除工作区时仍清理老安装里的历史数据（表存在时），但不再参与 legacy 工作区归属。
func TestRetiredWorkspaceDeleteStatementsCoverRemovedFeatureTables(t *testing.T) {
	want := []string{
		"lottery_reward_jobs",
		"lottery_winners",
		"lottery_draws",
		"lottery_entries",
		"lottery_prizes",
		"lottery_audit_logs",
		"lottery_campaigns",
		"lottery_embed_configs",
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
		"real_connections",
	}
	assertWorkspaceDeleteStatements(t, retiredWorkspaceDeleteStatements, want)

	active := map[string]bool{}
	for _, stmt := range workspaceDeleteStatements {
		active[stmt.Name] = true
	}
	legacy := map[string]bool{}
	for _, table := range legacyWorkspaceTables {
		legacy[table.Name] = true
	}
	for _, stmt := range retiredWorkspaceDeleteStatements {
		if active[stmt.Name] {
			t.Fatalf("%s is listed as both active and retired", stmt.Name)
		}
		if legacy[stmt.Name] {
			t.Fatalf("retired table %s must not create or assign legacy workspaces", stmt.Name)
		}
	}
}

func assertWorkspaceDeleteStatements(t *testing.T, statements []workspaceDeleteStatement, want []string) {
	t.Helper()
	if len(statements) != len(want) {
		t.Fatalf("delete statement count = %d, want %d", len(statements), len(want))
	}
	for i, stmt := range statements {
		if stmt.Name != want[i] {
			t.Fatalf("delete statement order[%d] = %s, want %s", i, stmt.Name, want[i])
		}
		if !strings.Contains(stmt.SQL, "DELETE FROM "+stmt.Name+" ") {
			t.Fatalf("%s delete targets another table: %s", stmt.Name, stmt.SQL)
		}
		if !strings.Contains(stmt.SQL, "user_id = $1") {
			t.Fatalf("%s delete is not scoped by user_id: %s", stmt.Name, stmt.SQL)
		}
		workspaceColumn := "admin_account_id = $2"
		if stmt.Name == "real_connections" {
			workspaceColumn = "workspace_admin_account_id = $2"
		}
		if !strings.Contains(stmt.SQL, workspaceColumn) {
			t.Fatalf("%s delete is not scoped by workspace column %q: %s", stmt.Name, workspaceColumn, stmt.SQL)
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
		if column != "admin_account_id" {
			t.Fatalf("legacy descriptor column for %s = %s, want admin_account_id", stmt.Name, column)
		}
	}
	if len(descriptors) != len(workspaceDeleteStatements) {
		t.Fatalf("legacy descriptors (%d) must match active delete tables (%d)", len(descriptors), len(workspaceDeleteStatements))
	}
}

func TestWorkspaceDeleteStatementsDeleteChildrenBeforeParents(t *testing.T) {
	order := map[string]int{}
	for i, stmt := range workspaceDeleteStatements {
		order[stmt.Name] = i
	}
	retiredOrder := map[string]int{}
	for i, stmt := range retiredWorkspaceDeleteStatements {
		retiredOrder[stmt.Name] = i
	}
	assertBefore := func(order map[string]int, child string, parent string) {
		t.Helper()
		if order[child] >= order[parent] {
			t.Fatalf("%s must be deleted before %s", child, parent)
		}
	}

	assertBefore(order, "mass_email_batch_items", "mass_email_batches")
	assertBefore(retiredOrder, "ticket_attachments", "ticket_messages")
	assertBefore(retiredOrder, "ticket_messages", "tickets")
	assertBefore(retiredOrder, "group_rate_campaign_items", "group_rate_campaigns")
	assertBefore(retiredOrder, "connection_health_group_policy_assignments", "connection_health_policies")
	assertBefore(retiredOrder, "connection_health_policy_assignments", "connection_health_policies")
	assertBefore(retiredOrder, "connection_health_model_targets", "connection_health_policies")
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
