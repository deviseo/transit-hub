package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v0.1.15", 1},
		{"v0.1.15", "v1.0.0", -1},
		{"v1.2.3", "1.2.3", 0},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0-rc1", "v2.0.0", 0},
		{"dev", "v0.0.1", -1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDecide(t *testing.T) {
	installed := func(version, status string) *State {
		return &State{Version: version, Status: status, Dir: "/app/data/updates/" + version}
	}
	cases := []struct {
		name    string
		state   *State
		current string
		want    decision
	}{
		{"nothing installed", nil, "v1.0.0", stay},
		{"newer pending update", installed("v1.1.0", StatusPending), "v1.0.0", switchTo},
		{"newer active update after a restart", installed("v1.1.0", StatusActive), "v1.0.0", switchTo},
		{"the update itself starting", installed("v1.1.0", StatusTrying), "v1.1.0", stay},
		{"previous switch never confirmed", installed("v1.1.0", StatusTrying), "v1.0.0", giveUp},
		{"failed update", installed("v1.1.0", StatusFailed), "v1.0.0", stay},
		{"newer image than the update", installed("v1.1.0", StatusActive), "v1.2.0", stay},
	}
	for _, tc := range cases {
		if got := decide(tc.state, tc.current); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestStateRoundTripAndMarkActive(t *testing.T) {
	dataDir := t.TempDir()
	if state, err := ReadState(dataDir); err != nil || state != nil {
		t.Fatalf("empty: %v %+v", err, state)
	}
	state := State{Version: "v1.1.0", Status: StatusTrying, Dir: filepath.Join(Dir(dataDir), "v1.1.0"), InstalledAt: time.Now().UTC()}
	if err := WriteState(dataDir, state); err != nil {
		t.Fatal(err)
	}
	MarkActive(dataDir, "v1.0.0") // 别的版本不能标记
	if got, _ := ReadState(dataDir); got.Status != StatusTrying {
		t.Fatalf("other versions must not mark active: %+v", got)
	}
	MarkActive(dataDir, "v1.1.0")
	if got, _ := ReadState(dataDir); got.Status != StatusActive {
		t.Fatalf("not active: %+v", got)
	}
}

func TestLaunchGivesUpOnAnUnconfirmedSwitch(t *testing.T) {
	dataDir := t.TempDir()
	_ = WriteState(dataDir, State{Version: "v1.1.0", Status: StatusTrying, Dir: filepath.Join(Dir(dataDir), "v1.1.0")})
	Launch(dataDir, "v1.0.0")
	if got, _ := ReadState(dataDir); got.Status != StatusFailed {
		t.Fatalf("expected failed, got %+v", got)
	}
}

func TestSwitchRefusesMissingOrNonExecutableBinaries(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(Dir(dataDir), "v1.1.0")
	state := State{Version: "v1.1.0", Status: StatusPending, Dir: dir}
	if err := Switch(dataDir, state); err == nil {
		t.Fatalf("missing binary should fail")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, BinaryName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Switch(dataDir, state); err == nil {
		t.Fatalf("non-executable binary should fail")
	}
	// 还没切换就失败：记录不动。
	if got, _ := ReadState(dataDir); got != nil {
		t.Fatalf("state should be untouched: %+v", got)
	}
}

// 在子进程里调用 Launch：切换到一个假的新版本（shell 脚本），脚本记下拿到的前端目录；
// 父进程检查脚本确实运行了、PUBLIC_DIR 指向哪个版本，以及切换后的记录。
func TestLaunchExecsTheInstalledVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("online update only runs on unix")
	}
	if os.Getenv("SELFUPDATE_HELPER") == "1" {
		Launch(os.Getenv("SELFUPDATE_DATA"), "v1.0.0")
		os.Exit(3) // 没有切换过去
	}
	cases := []struct {
		name  string
		state func(dirs map[string]string) State
		ran   string // 应该运行的版本
		after State  // 切换后的记录（只比较版本和状态）
	}{
		{
			name: "first switch waits for confirmation",
			state: func(dirs map[string]string) State {
				return State{Version: "v1.1.0", Status: StatusPending, Dir: dirs["v1.1.0"]}
			},
			ran:   "v1.1.0",
			after: State{Version: "v1.1.0", Status: StatusTrying},
		},
		{
			name: "confirmed version stays active",
			state: func(dirs map[string]string) State {
				return State{Version: "v1.1.0", Status: StatusActive, Dir: dirs["v1.1.0"]}
			},
			ran:   "v1.1.0",
			after: State{Version: "v1.1.0", Status: StatusActive},
		},
		{
			name: "failed first start goes back to the previous online version",
			state: func(dirs map[string]string) State {
				return State{Version: "v1.2.0", Status: StatusTrying, Dir: dirs["v1.2.0"], Previous: &State{Version: "v1.1.0", Status: StatusActive, Dir: dirs["v1.1.0"]}}
			},
			ran:   "v1.1.0",
			after: State{Version: "v1.1.0", Status: StatusActive},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			marker := filepath.Join(dataDir, "marker")
			dirs := map[string]string{}
			for _, version := range []string{"v1.1.0", "v1.2.0"} {
				dir := filepath.Join(Dir(dataDir), version)
				dirs[version] = dir
				if err := os.MkdirAll(filepath.Join(dir, PublicDirName), 0o755); err != nil {
					t.Fatal(err)
				}
				script := "#!/bin/sh\necho \"$PUBLIC_DIR\" > '" + marker + "'\n"
				if err := os.WriteFile(filepath.Join(dir, BinaryName), []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteState(dataDir, tc.state(dirs)); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchExecsTheInstalledVersion$")
			cmd.Env = append(os.Environ(), "SELFUPDATE_HELPER=1", "SELFUPDATE_DATA="+dataDir, "PUBLIC_DIR=/app/public")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("helper did not switch: %v\n%s", err, output)
			}
			got, err := os.ReadFile(marker)
			if err != nil {
				t.Fatalf("no version ran: %v", err)
			}
			if strings.TrimSpace(string(got)) != filepath.Join(dirs[tc.ran], PublicDirName) {
				t.Fatalf("PUBLIC_DIR = %q, want %s", got, tc.ran)
			}
			if state, _ := ReadState(dataDir); state == nil || state.Version != tc.after.Version || state.Status != tc.after.Status {
				t.Fatalf("expected %s %s after switching, got %+v", tc.after.Version, tc.after.Status, state)
			}
		})
	}
}
