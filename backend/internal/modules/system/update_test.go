package system

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"transithub/backend/internal/config"
	"transithub/backend/internal/selfupdate"
)

type updateAdmins map[string]bool

func (f updateAdmins) IsSystemAdmin(_ context.Context, userID string) (bool, error) {
	return f[userID], nil
}

type packageEntry struct {
	name string
	body string
	mode int64
	kind byte
}

func makePackage(t *testing.T, entries []packageEntry) []byte {
	t.Helper()
	return makePackageForTest(entries)
}

var goodPackage = []packageEntry{
	{name: "transithub-api", body: "#!/bin/sh\necho new\n", mode: 0o755},
	{name: "public/index.html", body: "<!doctype html>", mode: 0o644},
	{name: "public/assets/app.js", body: "console.log(1)", mode: 0o644},
}

// fakeGitHub 模拟最新正式版和它的下载文件。
type fakeGitHub struct {
	server       *httptest.Server
	pkg          []byte
	checksum     string
	assetURL     string
	releaseCalls atomic.Int32
}

func newFakeGitHub(t *testing.T, pkg []byte) *fakeGitHub {
	t.Helper()
	fake := &fakeGitHub{pkg: pkg}
	sum := sha256.Sum256(pkg)
	fake.checksum = hex.EncodeToString(sum[:])
	name := packageName("v1.1.0")
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fake.releaseCalls.Add(1)
			assetURL := fake.assetURL
			if assetURL == "" {
				assetURL = fake.server.URL + "/download/v1.1.0/" + name
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v1.1.0", "body": "## 新功能\n- 在线更新", "html_url": "https://github.com/deviseo/transit-hub/releases/tag/v1.1.0",
				"published_at": "2026-11-20T08:00:00Z", "draft": false, "prerelease": false,
				"assets": []map[string]any{
					{"name": name, "browser_download_url": assetURL, "size": len(fake.pkg)},
					{"name": "checksums.txt", "browser_download_url": fake.server.URL + "/download/v1.1.0/checksums.txt", "size": 100},
				},
			})
		case "/download/v1.1.0/" + name:
			_, _ = w.Write(fake.pkg)
		case "/download/v1.1.0/checksums.txt":
			_, _ = w.Write([]byte(fake.checksum + "  " + name + "\n" + "0000  other.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// newUpdateService 返回指向假 GitHub 的服务；切换进程换成把记录发到 switched（测试里不能真的 exec）。
func newUpdateService(t *testing.T, fake *fakeGitHub, current string) (*Service, chan selfupdate.State) {
	t.Helper()
	service := NewService(config.Config{AppVersion: current, DataDir: t.TempDir()})
	service.admins = updateAdmins{"admin": true}
	service.updates.url = fake.server.URL + "/latest"
	service.updates.downloadPrefix = fake.server.URL + "/download/"
	service.updates.supported = func() bool { return true }
	switched := make(chan selfupdate.State, 1)
	service.updates.switchTo = func(_ string, state selfupdate.State) error {
		switched <- state
		return nil
	}
	return service, switched
}

// waitForProgress 等后台更新走到 done 认可的进度。
func waitForProgress(t *testing.T, service *Service, done func(UpdateProgress) bool) UpdateProgress {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		progress := service.UpdateProgress()
		if done(progress) {
			return progress
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the update, progress %+v", progress)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func failed(progress UpdateProgress) bool { return progress.Status == UpdateStatusFailed }

func TestCheckUpdateReportsWhetherTheAdminCanUpdate(t *testing.T) {
	fake := newFakeGitHub(t, makePackage(t, goodPackage))
	service, _ := newUpdateService(t, fake, "v1.0.0")
	ctx := context.Background()

	result, err := service.CheckUpdate(ctx, "admin", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasUpdate || !result.CanUpdate || result.Reason != "" || result.Latest != "v1.1.0" || result.Notes == "" || result.PublishedAt == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result, _ := service.CheckUpdate(ctx, "someone", false); result.CanUpdate || result.Reason != UpdateReasonNotAdmin {
		t.Fatalf("non-admins cannot update: %+v", result)
	}
	// 10 分钟内再查用缓存；force 时重新请求。
	if fake.releaseCalls.Load() != 1 {
		t.Fatalf("expected cached result, calls = %d", fake.releaseCalls.Load())
	}
	_, _ = service.CheckUpdate(ctx, "admin", true)
	if fake.releaseCalls.Load() != 2 {
		t.Fatalf("force should refetch, calls = %d", fake.releaseCalls.Load())
	}

	service.updates.supported = func() bool { return false }
	if result, _ := service.CheckUpdate(ctx, "admin", false); result.Reason != UpdateReasonUnsupported {
		t.Fatalf("expected unsupported: %+v", result)
	}

	upToDate, _ := newUpdateService(t, fake, "v1.1.0")
	if result, _ := upToDate.CheckUpdate(ctx, "admin", false); result.HasUpdate || result.CanUpdate {
		t.Fatalf("same version should not report an update: %+v", result)
	}
}

func TestCheckUpdateReportsFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 例如 GitHub 限流
	}))
	defer server.Close()
	service := NewService(config.Config{AppVersion: "v1.0.0"})
	service.updates.url = server.URL
	if _, err := service.CheckUpdate(context.Background(), "admin", true); err == nil || err.Error() != ErrorUpdateCheckFailed {
		t.Fatalf("expected check failure, got %v", err)
	}
}

func TestApplyUpdateInstallsAVerifiedPackage(t *testing.T) {
	pkg := makePackage(t, goodPackage)
	fake := newFakeGitHub(t, pkg)
	service, switched := newUpdateService(t, fake, "v1.0.0")
	ctx := context.Background()

	if progress := service.UpdateProgress(); progress.Status != UpdateStatusIdle || progress.Version != "v1.0.0" {
		t.Fatalf("unexpected initial progress: %+v", progress)
	}
	if _, err := service.ApplyUpdate(ctx, "someone"); err == nil || err.Error() != ErrorUpdateForbidden {
		t.Fatalf("non-admins are refused, got %v", err)
	}
	// 留一个更早的版本，装好新版本后应被清理。
	oldDir := filepath.Join(selfupdate.Dir(service.cfg.DataDir), "v0.9.0")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 先检查更新（管理员看到弹窗），再开始更新：沿用检查的结果，不再请求一次 GitHub。
	if _, err := service.CheckUpdate(ctx, "admin", false); err != nil {
		t.Fatal(err)
	}
	version, err := service.ApplyUpdate(ctx, "admin")
	if err != nil || version != "v1.1.0" {
		t.Fatalf("expected the update to start, got %q %v", version, err)
	}
	if calls := fake.releaseCalls.Load(); calls != 1 {
		t.Fatalf("the update should reuse the fresh check, GitHub was asked %d times", calls)
	}
	// 更新进行中（到切换为止）不能再开始一次。
	if _, err := service.ApplyUpdate(ctx, "admin"); err == nil || err.Error() != ErrorUpdateInProgress {
		t.Fatalf("expected in progress, got %v", err)
	}
	var state selfupdate.State
	select {
	case state = <-switched:
	case <-time.After(5 * time.Second):
		t.Fatalf("never switched, progress %+v", service.UpdateProgress())
	}
	if state.Version != "v1.1.0" || state.Status != selfupdate.StatusPending {
		t.Fatalf("unexpected state: %+v", state)
	}
	progress := service.UpdateProgress()
	if progress.Status != UpdateStatusRestarting || progress.Target != "v1.1.0" || progress.Downloaded != int64(len(pkg)) || progress.Total != int64(len(pkg)) {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	saved, _ := selfupdate.ReadState(service.cfg.DataDir)
	if saved == nil || saved.Dir != state.Dir {
		t.Fatalf("state not saved: %+v", saved)
	}
	info, err := os.Stat(filepath.Join(state.Dir, selfupdate.BinaryName))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("binary missing or not executable: %v %v", err, info)
	}
	if _, err := os.Stat(filepath.Join(state.Dir, "public", "assets", "app.js")); err != nil {
		t.Fatalf("frontend files missing: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("older versions should be pruned")
	}
	if matches, _ := filepath.Glob(filepath.Join(selfupdate.Dir(service.cfg.DataDir), "*.download")); len(matches) != 0 {
		t.Fatalf("download leftovers: %v", matches)
	}
}

func TestApplyUpdateRemembersTheRunningOnlineVersion(t *testing.T) {
	fake := newFakeGitHub(t, makePackage(t, goodPackage))
	service, switched := newUpdateService(t, fake, "v1.0.5")
	// 正在运行的 v1.0.5 是之前在线更新装的。
	runningDir := filepath.Join(selfupdate.Dir(service.cfg.DataDir), "v1.0.5")
	if err := os.MkdirAll(runningDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := selfupdate.WriteState(service.cfg.DataDir, selfupdate.State{Version: "v1.0.5", Status: selfupdate.StatusActive, Dir: runningDir}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyUpdate(context.Background(), "admin"); err != nil {
		t.Fatal(err)
	}
	var state selfupdate.State
	select {
	case state = <-switched:
	case <-time.After(5 * time.Second):
		t.Fatalf("never switched, progress %+v", service.UpdateProgress())
	}
	if state.Previous == nil || state.Previous.Version != "v1.0.5" || state.Previous.Dir != runningDir || state.Previous.Previous != nil {
		t.Fatalf("previous version not recorded: %+v", state.Previous)
	}
	if _, err := os.Stat(runningDir); err != nil {
		t.Fatalf("the running version must not be pruned: %v", err)
	}
}

func TestApplyUpdateOnlyInstallsPlainVersionTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v1.1.0/../../evil", "html_url": "x",
			"assets": []map[string]any{{"name": "checksums.txt", "browser_download_url": "x"}},
		})
	}))
	defer server.Close()
	service := NewService(config.Config{AppVersion: "v1.0.0", DataDir: t.TempDir()})
	service.admins = updateAdmins{"admin": true}
	service.updates.url = server.URL
	service.updates.supported = func() bool { return true }
	if result, err := service.CheckUpdate(context.Background(), "admin", true); err != nil || result.CanUpdate || result.Reason != UpdateReasonNoPackage {
		t.Fatalf("odd tags cannot be installed: %+v %v", result, err)
	}
	if _, err := service.ApplyUpdate(context.Background(), "admin"); err == nil || err.Error() != ErrorUpdateUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestApplyUpdateRefusesWhenThereIsNothingToInstall(t *testing.T) {
	fake := newFakeGitHub(t, makePackage(t, goodPackage))
	service, _ := newUpdateService(t, fake, "v1.1.0")
	if _, err := service.ApplyUpdate(context.Background(), "admin"); err == nil || err.Error() != ErrorUpdateUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if progress := service.UpdateProgress(); progress.Status != UpdateStatusIdle {
		t.Fatalf("progress should go back to idle: %+v", progress)
	}
}

func TestApplyUpdateReportsAFailedSwitch(t *testing.T) {
	fake := newFakeGitHub(t, makePackage(t, goodPackage))
	service, _ := newUpdateService(t, fake, "v1.0.0")
	service.updates.switchTo = func(string, selfupdate.State) error { return os.ErrPermission }
	if _, err := service.ApplyUpdate(context.Background(), "admin"); err != nil {
		t.Fatal(err)
	}
	if progress := waitForProgress(t, service, failed); progress.Error != ErrorUpdateFailed {
		t.Fatalf("unexpected progress: %+v", progress)
	}
}

func TestApplyUpdateRejectsBadPackages(t *testing.T) {
	cases := map[string]func(*fakeGitHub){
		"wrong checksum": func(f *fakeGitHub) { f.checksum = "aa" + f.checksum[2:] },
		"path traversal": func(f *fakeGitHub) {
			f.pkg = makePackageForTest(append([]packageEntry{{name: "../evil", body: "x", mode: 0o644}}, goodPackage...))
			sum := sha256.Sum256(f.pkg)
			f.checksum = hex.EncodeToString(sum[:])
		},
		"symlink": func(f *fakeGitHub) {
			f.pkg = makePackageForTest(append([]packageEntry{{name: "public/passwd", kind: tar.TypeSymlink}}, goodPackage...))
			sum := sha256.Sum256(f.pkg)
			f.checksum = hex.EncodeToString(sum[:])
		},
		"missing binary": func(f *fakeGitHub) {
			f.pkg = makePackageForTest(goodPackage[1:])
			sum := sha256.Sum256(f.pkg)
			f.checksum = hex.EncodeToString(sum[:])
		},
		"foreign download url": func(f *fakeGitHub) { f.assetURL = "https://evil.example.com/transithub.tar.gz" },
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeGitHub(t, makePackage(t, goodPackage))
			tamper(fake)
			service, switched := newUpdateService(t, fake, "v1.0.0")
			ctx := context.Background()
			if _, err := service.ApplyUpdate(ctx, "admin"); err != nil {
				t.Fatal(err)
			}
			if progress := waitForProgress(t, service, failed); progress.Error != ErrorUpdateFailed {
				t.Fatalf("unexpected progress: %+v", progress)
			}
			if len(switched) != 0 {
				t.Fatalf("a bad package must never be switched to")
			}
			if state, _ := selfupdate.ReadState(service.cfg.DataDir); state != nil {
				t.Fatalf("nothing should be recorded: %+v", state)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(service.cfg.DataDir), "evil")); err == nil {
				t.Fatalf("file escaped the update directory")
			}
			// 失败后可以重试。
			if _, err := service.ApplyUpdate(ctx, "admin"); err != nil {
				t.Fatalf("should be able to retry after a failure: %v", err)
			}
			waitForProgress(t, service, failed)
		})
	}
}

// makePackageForTest 是不依赖 *testing.T 的打包（给 tamper 回调用）。
func makePackageForTest(entries []packageEntry) []byte {
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.body)), Typeflag: kind}
		if kind == tar.TypeSymlink {
			header.Size = 0
			header.Linkname = "/etc/passwd"
		}
		_ = tw.WriteHeader(header)
		if kind == tar.TypeReg {
			_, _ = tw.Write([]byte(entry.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buffer.Bytes()
}
