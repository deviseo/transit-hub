package system

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"transithub/backend/internal/selfupdate"
	"transithub/backend/internal/shared/datadir"
)

const (
	// latestReleaseURL 是 GitHub 上最新正式版的接口（不含草稿和预发布版）。
	latestReleaseURL = "https://api.github.com/repos/deviseo/transit-hub/releases/latest"
	// releaseDownloadPrefix：更新包只能从这个仓库的 Release 下载。
	releaseDownloadPrefix = "https://github.com/deviseo/transit-hub/releases/download/"

	// updateCheckTTL 内重复检查直接用上次的结果，避免频繁访问 GitHub（未登录的接口每小时限 60 次）。
	updateCheckTTL        = 10 * time.Minute
	updateCheckTimeout    = 15 * time.Second
	updateDownloadTimeout = 10 * time.Minute
	// 切换前等一会儿，让查询进度的页面看到「正在重启」。
	updateSwitchDelay = 1500 * time.Millisecond

	maxReleaseNotes   = 8 * 1024
	maxChecksumsBytes = 64 * 1024
	maxPackageBytes   = 300 << 20
	maxExtractedBytes = 600 << 20
	maxExtractedFiles = 5000

	ErrorUpdateCheckFailed = "admin.update.errors.checkFailed"
	ErrorUpdateForbidden   = "admin.update.errors.forbidden"
	ErrorUpdateInProgress  = "admin.update.errors.inProgress"
	ErrorUpdateUnavailable = "admin.update.errors.unavailable"
	ErrorUpdateFailed      = "admin.update.errors.failed"

	// 不能在线更新的原因（有新版本时才有）。
	UpdateReasonNotAdmin    = "notAdmin"
	UpdateReasonUnsupported = "unsupported"
	UpdateReasonNoPackage   = "noPackage"

	// 在线更新的进度。
	UpdateStatusIdle        = "idle"
	UpdateStatusDownloading = "downloading"
	UpdateStatusInstalling  = "installing"
	UpdateStatusRestarting  = "restarting"
	UpdateStatusFailed      = "failed"

	checksumsAssetName = "checksums.txt"
)

// UpdateCheckResponse 是 GET /api/system/update 的响应。
type UpdateCheckResponse struct {
	Current     string     `json:"current"`
	Latest      string     `json:"latest"`
	HasUpdate   bool       `json:"hasUpdate"`
	Notes       string     `json:"notes"`
	PublishedAt *time.Time `json:"publishedAt"`
	ReleaseURL  string     `json:"releaseUrl"`
	CheckedAt   time.Time  `json:"checkedAt"`
	// CanUpdate：可以在线更新（有新版本、是系统管理员、部署环境支持、Release 里有对应的更新包）。
	CanUpdate bool `json:"canUpdate"`
	// Reason 说明不能在线更新的原因：notAdmin、unsupported、noPackage。
	Reason string `json:"reason,omitempty"`
	// Persistent：数据目录是持久卷（在线更新的版本和凭据密钥重建容器后还在）。
	Persistent bool `json:"persistent"`
	// CredentialsEncrypted：保存的登录凭据是加密的。
	CredentialsEncrypted bool `json:"credentialsEncrypted"`
}

// ApplyUpdateResponse 是 POST /api/system/update 的响应：已开始在后台更新到这个版本。
type ApplyUpdateResponse struct {
	Version string `json:"version"`
}

// UpdateProgress 是 GET /api/system/update/progress 的响应：正在运行的版本和在线更新的进度。
// 服务重启后进度从 idle 重新开始，页面据此判断新版本有没有启动起来。
type UpdateProgress struct {
	Version    string `json:"version"`
	Status     string `json:"status"`
	Target     string `json:"target,omitempty"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error,omitempty"`
}

// updateChecker 缓存最近一次检查的结果，并记录在线更新的进度（同一时间只有一次在线更新）。
type updateChecker struct {
	mu        sync.Mutex
	latest    *githubRelease
	checkedAt time.Time

	progressMu sync.Mutex
	status     string
	target     string
	total      int64
	failure    string
	downloaded atomic.Int64

	// 测试用：替换 GitHub 地址、下载地址前缀、「部署环境是否支持」的判断和切换进程。
	url            string
	downloadPrefix string
	supported      func() bool
	switchTo       func(dataDir string, state selfupdate.State) error
	switchDelay    time.Duration
}

// begin 占住「正在更新」；已经在更新时返回 false。
func (c *updateChecker) begin() bool {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	switch c.status {
	case UpdateStatusDownloading, UpdateStatusInstalling, UpdateStatusRestarting:
		return false
	}
	c.status, c.target, c.total, c.failure = UpdateStatusDownloading, "", 0, ""
	c.downloaded.Store(0)
	return true
}

func (c *updateChecker) set(status string, target string, total int64) {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	c.status = status
	if target != "" {
		c.target, c.total = target, total
	}
}

// finish 结束这次更新：failure 为空表示没开始就退出（例如检查版本失败），回到 idle。
func (c *updateChecker) finish(failure string) {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	if failure == "" {
		c.status, c.target, c.total = UpdateStatusIdle, "", 0
		return
	}
	c.status, c.failure = UpdateStatusFailed, failure
}

func (c *updateChecker) snapshot() UpdateProgress {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	status := c.status
	if status == "" {
		status = UpdateStatusIdle
	}
	return UpdateProgress{Status: status, Target: c.target, Downloaded: c.downloaded.Load(), Total: c.total, Error: c.failure}
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt *time.Time    `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func (r githubRelease) asset(name string) *githubAsset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// releaseTag：只在线安装 vX.Y.Z 形式的正式版本（版本号会用在文件路径里）。
var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// packageName 是当前平台的更新包文件名，例如 transithub_v1.1.0_linux_amd64.tar.gz。
func packageName(version string) string {
	return fmt.Sprintf("transithub_%s_linux_%s.tar.gz", version, runtime.GOARCH)
}

// CheckUpdate 对比当前版本和 GitHub 上最新的正式版，并说明能不能在线更新。force 为真时不用缓存。
func (s *Service) CheckUpdate(ctx context.Context, userID string, force bool) (UpdateCheckResponse, error) {
	release, checkedAt, err := s.latestRelease(ctx, force)
	if err != nil {
		return UpdateCheckResponse{}, err
	}
	notes := strings.TrimSpace(release.Body)
	if len(notes) > maxReleaseNotes {
		notes = notes[:maxReleaseNotes]
	}
	result := UpdateCheckResponse{
		Current:     s.cfg.AppVersion,
		Latest:      release.TagName,
		HasUpdate:   selfupdate.CompareVersions(release.TagName, s.cfg.AppVersion) > 0,
		Notes:       notes,
		PublishedAt: release.PublishedAt,
		ReleaseURL:  release.HTMLURL,
		CheckedAt:   checkedAt,

		Persistent:           datadir.Persistent(s.cfg.DataDir),
		CredentialsEncrypted: s.credentialsEncrypted,
	}
	if result.HasUpdate {
		result.Reason = s.updateBlocker(ctx, userID, release)
		result.CanUpdate = result.Reason == ""
	}
	return result, nil
}

// updateBlocker 返回不能在线更新的原因；可以更新时返回空。
func (s *Service) updateBlocker(ctx context.Context, userID string, release githubRelease) string {
	switch {
	case !s.isSystemAdmin(ctx, userID):
		return UpdateReasonNotAdmin
	case !s.updateSupported():
		return UpdateReasonUnsupported
	case !releaseTag.MatchString(release.TagName) || release.asset(packageName(release.TagName)) == nil || release.asset(checksumsAssetName) == nil:
		return UpdateReasonNoPackage
	default:
		return ""
	}
}

func (s *Service) isSystemAdmin(ctx context.Context, userID string) bool {
	if s.admins == nil || userID == "" {
		return false
	}
	ok, err := s.admins.IsSystemAdmin(ctx, userID)
	return err == nil && ok
}

// updateSupported：只有 Linux（amd64、arm64）上运行、数据目录可写的发布版本能在线更新。
func (s *Service) updateSupported() bool {
	if s.updates.supported != nil {
		return s.updates.supported()
	}
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return false
	}
	return dirWritable(selfupdate.Dir(s.cfg.DataDir))
}

func dirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	file, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return false
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return true
}

// UpdateProgress 返回正在运行的版本和在线更新的进度。
func (s *Service) UpdateProgress() UpdateProgress {
	progress := s.updates.snapshot()
	progress.Version = s.cfg.AppVersion
	return progress
}

// ApplyUpdate 开始在线更新：确认有可以安装的新版本后，在后台下载、校验、安装并切换过去，马上返回目标版本
// （下载可能要一两分钟，不让请求一直等着，免得被反向代理超时断开）。进度用 UpdateProgress 查询。
// 只有系统管理员能操作；同一时间只能有一次更新。
func (s *Service) ApplyUpdate(ctx context.Context, userID string) (string, error) {
	if !s.isSystemAdmin(ctx, userID) {
		return "", requestError(ErrorUpdateForbidden)
	}
	if !s.updates.begin() {
		return "", requestError(ErrorUpdateInProgress)
	}
	// 用刚才检查更新的结果（10 分钟内有效），不再多请求一次 GitHub（国内直连 GitHub 时常不稳定）。
	release, _, err := s.latestRelease(ctx, false)
	if err != nil {
		s.updates.finish("")
		return "", err
	}
	version := release.TagName
	pkg := release.asset(packageName(version))
	sums := release.asset(checksumsAssetName)
	if !releaseTag.MatchString(version) || selfupdate.CompareVersions(version, s.cfg.AppVersion) <= 0 || !s.updateSupported() || pkg == nil || sums == nil {
		s.updates.finish("")
		return "", requestError(ErrorUpdateUnavailable)
	}
	s.updates.set(UpdateStatusDownloading, version, pkg.Size)
	go s.runUpdate(version, *pkg, *sums)
	return version, nil
}

// runUpdate 在后台安装并切换到新版本（切换成功时进程被替换，不会返回）。
func (s *Service) runUpdate(version string, pkg githubAsset, sums githubAsset) {
	ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
	defer cancel()
	state, err := s.install(ctx, version, pkg, sums)
	if err != nil {
		s.updates.finish(ErrorUpdateFailed)
		return
	}
	s.updates.set(UpdateStatusRestarting, "", 0)
	delay, switchTo := updateSwitchDelay, selfupdate.Switch
	if s.updates.switchTo != nil {
		delay, switchTo = s.updates.switchDelay, s.updates.switchTo
	}
	time.Sleep(delay)
	if err := switchTo(s.cfg.DataDir, state); err != nil {
		log.Printf("[update] switch to %s failed: %v", version, err)
		s.updates.finish(ErrorUpdateFailed)
	}
}

// install 下载、校验并解压新版本，写好更新记录。
func (s *Service) install(ctx context.Context, version string, pkg githubAsset, sums githubAsset) (selfupdate.State, error) {
	fail := func(step string, err error) (selfupdate.State, error) {
		log.Printf("[update] %s %s: %v", step, version, err)
		return selfupdate.State{}, err
	}
	expected, err := s.fetchChecksum(ctx, sums.URL, pkg.Name)
	if err != nil {
		return fail("checksum list", err)
	}
	updatesDir := selfupdate.Dir(s.cfg.DataDir)
	if err := os.MkdirAll(updatesDir, 0o755); err != nil {
		return fail("prepare", err)
	}
	archive := filepath.Join(updatesDir, version+".tar.gz.download")
	defer os.Remove(archive)
	actual, err := s.download(ctx, pkg.URL, archive, maxPackageBytes)
	if err != nil {
		return fail("download", err)
	}
	if actual != expected {
		return fail("verify", fmt.Errorf("sha256 %s, expected %s", actual, expected))
	}

	s.updates.set(UpdateStatusInstalling, "", 0)
	target := filepath.Join(updatesDir, version)
	temp := target + ".tmp"
	_ = os.RemoveAll(temp)
	if err := extractPackage(archive, temp); err != nil {
		_ = os.RemoveAll(temp)
		return fail("extract", err)
	}
	binary := filepath.Join(temp, selfupdate.BinaryName)
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		_ = os.RemoveAll(temp)
		return fail("extract", errors.New("package has no "+selfupdate.BinaryName))
	}
	if _, err := os.Stat(filepath.Join(temp, selfupdate.PublicDirName, "index.html")); err != nil {
		_ = os.RemoveAll(temp)
		return fail("extract", errors.New("package has no public/index.html"))
	}
	if err := os.Chmod(binary, 0o755); err != nil {
		_ = os.RemoveAll(temp)
		return fail("extract", err)
	}
	_ = os.RemoveAll(target)
	if err := os.Rename(temp, target); err != nil {
		_ = os.RemoveAll(temp)
		return fail("install", err)
	}
	state := selfupdate.State{Version: version, Status: selfupdate.StatusPending, Dir: target, InstalledAt: time.Now().UTC()}
	// 正在运行的是之前在线更新装的版本：记下来，新版本第一次启动失败时退回它。
	if running, _ := selfupdate.ReadState(s.cfg.DataDir); running != nil && running.Version == s.cfg.AppVersion && running.Status == selfupdate.StatusActive {
		state.Previous = &selfupdate.State{Version: running.Version, Status: selfupdate.StatusActive, Dir: running.Dir, InstalledAt: running.InstalledAt}
	}
	if err := selfupdate.WriteState(s.cfg.DataDir, state); err != nil {
		return fail("record", err)
	}
	pruneUpdates(updatesDir, version, s.cfg.AppVersion)
	log.Printf("[update] installed %s", version)
	return state, nil
}

// pruneUpdates 只留新装的版本和正在运行的版本，删掉更早的版本和下载残留。
func pruneUpdates(updatesDir string, keep ...string) {
	entries, err := os.ReadDir(updatesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "state.json" || strings.HasPrefix(name, ".") {
			continue
		}
		kept := false
		for _, version := range keep {
			if name == version {
				kept = true
			}
		}
		if !kept {
			_ = os.RemoveAll(filepath.Join(updatesDir, name))
		}
	}
}

func (s *Service) latestRelease(ctx context.Context, force bool) (githubRelease, time.Time, error) {
	checker := &s.updates
	checker.mu.Lock()
	defer checker.mu.Unlock()
	if !force && checker.latest != nil && time.Since(checker.checkedAt) < updateCheckTTL {
		return *checker.latest, checker.checkedAt, nil
	}
	endpoint := checker.url
	if endpoint == "" {
		endpoint = latestReleaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, time.Time{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", s.userAgent())
	response, err := s.updateHTTPClient(updateCheckTimeout).Do(request)
	if err != nil {
		log.Printf("[update] check latest release: %v", err)
		return githubRelease{}, time.Time{}, requestError(ErrorUpdateCheckFailed)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		// GitHub 限流时返回 403/429，X-RateLimit-Remaining 为 0。
		log.Printf("[update] check latest release: HTTP %d, rate limit remaining %q, read error %v",
			response.StatusCode, response.Header.Get("X-RateLimit-Remaining"), err)
		return githubRelease{}, time.Time{}, requestError(ErrorUpdateCheckFailed)
	}
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil || release.TagName == "" || release.Draft || release.Prerelease {
		log.Printf("[update] check latest release: unexpected response (%v)", err)
		return githubRelease{}, time.Time{}, requestError(ErrorUpdateCheckFailed)
	}
	checker.latest = &release
	checker.checkedAt = time.Now()
	return release, checker.checkedAt, nil
}

func (s *Service) userAgent() string {
	return "TransitHub/" + strings.TrimPrefix(s.cfg.AppVersion, "v")
}

// updateHTTPClient 走系统设置里的出站代理（没有配置时直连）。跳转只允许 https，且只能去 GitHub 的下载域名。
func (s *Service) updateHTTPClient(timeout time.Duration) *http.Client {
	var client *http.Client
	if s.proxyController != nil {
		client = s.proxyController.NewClient(timeout)
	} else {
		client = &http.Client{Timeout: timeout}
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		host := request.URL.Hostname()
		if request.URL.Scheme != "https" || (host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com")) {
			return fmt.Errorf("redirect to %s is not allowed", request.URL.Host)
		}
		return nil
	}
	return client
}

// allowedDownload：更新包和校验文件只能来自这个仓库的 Release。
func (s *Service) allowedDownload(raw string) error {
	prefix := s.updates.downloadPrefix
	if prefix == "" {
		prefix = releaseDownloadPrefix
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || !strings.HasPrefix(raw, prefix) || strings.Contains(parsed.Path, "..") {
		return fmt.Errorf("download url %q is not allowed", raw)
	}
	return nil
}

func (s *Service) get(ctx context.Context, raw string) (*http.Response, error) {
	if err := s.allowedDownload(raw); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", s.userAgent())
	response, err := s.updateHTTPClient(updateDownloadTimeout).Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", raw, response.StatusCode)
	}
	return response, nil
}

// fetchChecksum 从 checksums.txt（`sha256sum` 格式）里找出某个文件的 SHA-256。
func (s *Service) fetchChecksum(ctx context.Context, raw string, fileName string) (string, error) {
	response, err := s.get(ctx, raw)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxChecksumsBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxChecksumsBytes {
		return "", errors.New("checksum list is too large")
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != fileName {
			continue
		}
		sum := strings.ToLower(fields[0])
		if decoded, err := hex.DecodeString(sum); err != nil || len(decoded) != sha256.Size {
			return "", errors.New("malformed checksum")
		}
		return sum, nil
	}
	return "", fmt.Errorf("no checksum for %s", fileName)
}

// download 下载到文件，边下边算 SHA-256 并记录进度；超过 limit 字节就放弃。
func (s *Service) download(ctx context.Context, raw string, dest string, limit int64) (string, error) {
	response, err := s.get(ctx, raw)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	file, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	counter := progressWriter{downloaded: &s.updates.downloaded}
	written, err := io.Copy(io.MultiWriter(file, hash, counter), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", err
	}
	if written > limit {
		return "", errors.New("package is too large")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// progressWriter 累计已下载的字节数。
type progressWriter struct{ downloaded *atomic.Int64 }

func (w progressWriter) Write(data []byte) (int, error) {
	w.downloaded.Add(int64(len(data)))
	return len(data), nil
}

// extractPackage 解压 tar.gz：只接受普通文件和目录，不允许绝对路径、`..`、链接，并限制大小和数量。
func extractPackage(archive string, dest string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	var total int64
	files := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe path %q", header.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			files++
			total += header.Size
			if files > maxExtractedFiles || total > maxExtractedBytes {
				return errors.New("package is too large")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if header.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, io.LimitReader(reader, header.Size))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported entry %q (type %s)", header.Name, strconv.Itoa(int(header.Typeflag)))
		}
	}
}
