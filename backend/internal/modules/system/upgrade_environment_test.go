package system

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceUpgradeProvidesExplicitGoBuildCacheEnvironment(t *testing.T) {
	unit := readProjectFileForUpgradeTest(t, "deploy/transithub-upgrade.service")
	for _, want := range []string{
		"Environment=HOME=/root",
		"Environment=GOPATH=/root/go",
		"Environment=GOMODCACHE=/root/go/pkg/mod",
		"Environment=GOCACHE=/root/.cache/go-build",
	} {
		requireTextContains(t, unit, want)
	}

	script := readProjectFileForUpgradeTest(t, "deploy/update-source.sh")
	for _, want := range []string{
		"prepare_go_environment()",
		`export HOME="${HOME:-/root}"`,
		`export GOPATH="${GOPATH:-$HOME/go}"`,
		`export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"`,
		`export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"`,
		`install -d -m 0755 "$GOPATH" "$GOMODCACHE" "$GOCACHE"`,
		"\nprepare_go_environment\n",
	} {
		requireTextContains(t, script, want)
	}
}

func TestSourceUpgradeWaitsForHealthAfterRestart(t *testing.T) {
	script := readProjectFileForUpgradeTest(t, "deploy/update-source.sh")
	for _, want := range []string{
		"wait_for_health()",
		"local attempts=60",
		`health_response="$(curl -fsS "$HEALTH_URL" 2>/dev/null)"`,
		"sleep 1",
		`health_response="$(wait_for_health)"`,
	} {
		requireTextContains(t, script, want)
	}

	requireTextNotContains(t, script, `health_response="$(curl -fsS "$HEALTH_URL")"`)
}

func TestServiceRestartUsesFixedSystemdUnitAndHealthContract(t *testing.T) {
	unit := readProjectFileForUpgradeTest(t, "deploy/transithub-restart.service")
	for _, want := range []string{
		"Type=oneshot",
		"WorkingDirectory=/opt/transithub",
		"ExecStart=/opt/transithub/run-service-restart.sh",
		"TimeoutStartSec=120",
	} {
		requireTextContains(t, unit, want)
	}

	script := readProjectFileForUpgradeTest(t, "deploy/run-service-restart.sh")
	for _, want := range []string{
		"SERVICE='transithub-api.service'",
		"HEALTH_URL='http://127.0.0.1:10621/api/health'",
		"HEALTH_TIMEOUT_SECONDS=60",
		"LOCK_FILE='/run/lock/transithub-maintenance.lock'",
		`systemctl restart "$SERVICE"`,
		`systemctl is-active --quiet "$SERVICE"`,
		`>>"$LOG_FILE" 2>&1`,
		`systemctl status "$SERVICE" --no-pager -l`,
		`journalctl -u "$SERVICE" -n 50 --no-pager`,
		`health_response="$(curl -fsS --max-time 2 "$HEALTH_URL" 2>&1)"`,
		"最后一次健康检查输出",
		"sleep 1",
		"restart-status.json",
		"restart.log",
		"flock -n 9",
	} {
		requireTextContains(t, script, want)
	}
}

func TestSourceUpgradeAndServiceRestartShareMaintenanceLock(t *testing.T) {
	upgradeScript := readProjectFileForUpgradeTest(t, "deploy/run-source-upgrade.sh")
	restartScript := readProjectFileForUpgradeTest(t, "deploy/run-service-restart.sh")
	for _, script := range []string{upgradeScript, restartScript} {
		requireTextContains(t, script, "LOCK_FILE='/run/lock/transithub-maintenance.lock'")
		requireTextContains(t, script, `if ! exec 9>"$LOCK_FILE"; then`)
		requireTextContains(t, script, "flock -n 9")
		requireTextContains(t, script, "无法打开维护锁文件")
	}
}

func readProjectFileForUpgradeTest(t *testing.T, relativePath string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../../../"))
	payload, err := os.ReadFile(filepath.Join(projectRoot, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(payload)
}

func requireTextContains(t *testing.T, text, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Fatalf("expected text to contain %q", want)
	}
}

func requireTextNotContains(t *testing.T, text, want string) {
	t.Helper()

	if strings.Contains(text, want) {
		t.Fatalf("expected text not to contain %q", want)
	}
}
