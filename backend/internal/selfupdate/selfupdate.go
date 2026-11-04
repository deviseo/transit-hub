// Package selfupdate 管理在线更新装好的新版本。
//
// 新版本（服务端程序 + 前端文件）解压在 <数据目录>/updates/<版本>/，state.json 记录装了哪个版本、
// 启动得怎么样：
//
//   - 装好后，正在运行的进程直接切换（exec）到新版本，不依赖容器的重启策略；
//   - 以后容器重启时，镜像自带的程序发现装了更新的版本，同样切换过去；
//   - 新版本第一次启动成功后自己标记 active；第一次切过去就没能标记（例如启动就崩溃），下次启动时
//     退回更新前运行的版本（之前在线更新装的版本，没有就是镜像自带的版本），不会一直重启失败。
//     已经 active 的版本以后启动失败（例如数据库暂时连不上）不会因此退回旧版本。
package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// StatusPending：刚装好，还没切换过去。
	StatusPending = "pending"
	// StatusTrying：第一次切换过去，等新版本确认启动成功。
	StatusTrying = "trying"
	// StatusActive：新版本至少成功启动过一次。
	StatusActive = "active"
	// StatusFailed：第一次切换后没能启动成功，不再使用。
	StatusFailed = "failed"

	// BinaryName 是发布包里服务端程序的文件名。
	BinaryName = "transithub-api"
	// PublicDirName 是发布包里前端文件的目录名。
	PublicDirName = "public"
)

// State 是 <数据目录>/updates/state.json。
type State struct {
	Version     string    `json:"version"`
	Status      string    `json:"status"`
	Dir         string    `json:"dir"`
	InstalledAt time.Time `json:"installedAt"`
	// Previous 是装这个版本时正在运行的在线更新版本（成功启动过）；这个版本第一次启动失败时退回它。
	Previous *State `json:"previous,omitempty"`
}

// Dir 返回存放更新的目录。
func Dir(dataDir string) string {
	return filepath.Join(dataDir, "updates")
}

func statePath(dataDir string) string {
	return filepath.Join(Dir(dataDir), "state.json")
}

// ReadState 读取更新记录；没有装过更新时返回 nil。
func ReadState(dataDir string) (*State, error) {
	data, err := os.ReadFile(statePath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteState 原子写入更新记录。
func WriteState(dataDir string, state State) error {
	if err := os.MkdirAll(Dir(dataDir), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temp := statePath(dataDir) + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, statePath(dataDir))
}

type decision int

const (
	stay     decision = iota // 用当前程序
	switchTo                 // 切换到已装的新版本
	giveUp                   // 上次切换后没启动成功：标记失败，用当前程序
)

// decide 决定启动时要不要切换到已装的版本。
func decide(state *State, current string) decision {
	switch {
	case state == nil || state.Version == "" || state.Dir == "":
		return stay
	case state.Version == current:
		// 就是已装的新版本自己在启动（启动成功后由它标记 active）。
		return stay
	case CompareVersions(state.Version, current) <= 0:
		// 镜像自带的版本不比它旧（例如已经换了更新的镜像）：用镜像自带的。
		return stay
	case state.Status == StatusFailed:
		return stay
	case state.Status == StatusTrying:
		// 第一次切过去后没能确认启动成功。
		return giveUp
	default:
		return switchTo
	}
}

// Launch 在启动最开始调用：装了比自己新的版本就切换过去（成功时不会返回）。
func Launch(dataDir string, current string) {
	state, err := ReadState(dataDir)
	if err != nil {
		log.Printf("[update] read state: %v", err)
		return
	}
	switch decide(state, current) {
	case giveUp:
		log.Printf("[update] %s did not start last time", state.Version)
		if previous := state.Previous; previous != nil && previous.Dir != "" && CompareVersions(previous.Version, current) > 0 {
			// 退回更新前运行的版本（它成功启动过，直接记成 active）。
			restored := State{Version: previous.Version, Status: StatusActive, Dir: previous.Dir, InstalledAt: previous.InstalledAt}
			if err := WriteState(dataDir, restored); err != nil {
				log.Printf("[update] restore %s: %v", restored.Version, err)
				return
			}
			if err := Switch(dataDir, restored); err != nil {
				log.Printf("[update] switch back to %s: %v; staying on %s", restored.Version, err, current)
			}
			return
		}
		state.Status = StatusFailed
		if err := WriteState(dataDir, *state); err != nil {
			log.Printf("[update] mark %s failed: %v", state.Version, err)
		}
		log.Printf("[update] staying on %s", current)
	case switchTo:
		if err := Switch(dataDir, *state); err != nil {
			log.Printf("[update] switch to %s: %v; staying on %s", state.Version, err, current)
		}
	}
}

// Switch 切换到已装的版本，用新程序替换当前进程（成功时不会返回）。
// 还没成功启动过的版本先把记录改成 trying，等它启动成功后自己改成 active。
func Switch(dataDir string, state State) error {
	binary := filepath.Join(state.Dir, BinaryName)
	info, err := os.Stat(binary)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not an executable file", binary)
	}
	confirmed := state.Status == StatusActive
	if !confirmed {
		state.Status = StatusTrying
		if err := WriteState(dataDir, state); err != nil {
			return err
		}
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "PUBLIC_DIR=") {
			env = append(env, item)
		}
	}
	env = append(env, "PUBLIC_DIR="+filepath.Join(state.Dir, PublicDirName))
	log.Printf("[update] switching to %s", state.Version)
	err = execProcess(binary, append([]string{binary}, os.Args[1:]...), env)
	// 走到这里说明切换失败，继续用当前程序；还没成功启动过的版本标记失败。
	if !confirmed {
		state.Status = StatusFailed
		_ = WriteState(dataDir, state)
	}
	return err
}

// MarkActive 由新版本在启动成功（数据库迁移完成、端口已监听）后调用；已经 active 时不用再写。
func MarkActive(dataDir string, current string) {
	state, err := ReadState(dataDir)
	if err != nil || state == nil || state.Version != current || state.Status == StatusActive {
		return
	}
	state.Status = StatusActive
	if err := WriteState(dataDir, *state); err != nil {
		log.Printf("[update] mark %s active: %v", current, err)
		return
	}
	log.Printf("[update] now running %s", current)
}

// CompareVersions 比较 v1.2.3 形式的版本号：a 新返回 1，相同返回 0，a 旧返回 -1。
// 预发布后缀（-rc1 等）忽略；无法解析的部分当作 0。
func CompareVersions(a string, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := range pa {
		switch {
		case pa[i] > pb[i]:
			return 1
		case pa[i] < pb[i]:
			return -1
		}
	}
	return 0
}

func parseVersion(version string) [3]int {
	var parts [3]int
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if index := strings.IndexAny(version, "-+"); index >= 0 {
		version = version[:index]
	}
	for i, field := range strings.SplitN(version, ".", 3) {
		parts[i], _ = strconv.Atoi(field)
	}
	return parts
}
