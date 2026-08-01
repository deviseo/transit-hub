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
