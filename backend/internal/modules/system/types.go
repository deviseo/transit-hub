package system

// VersionResponse GET /api/system/version 响应。
// 开源版只展示版本号，不再包含授权状态或更新开关字段。
type VersionResponse struct {
	Version string `json:"version"`
}

type UpgradeState string

const (
	UpgradeStateIdle      UpgradeState = "idle"
	UpgradeStateStarting  UpgradeState = "starting"
	UpgradeStateRunning   UpgradeState = "running"
	UpgradeStateSucceeded UpgradeState = "succeeded"
	UpgradeStateFailed    UpgradeState = "failed"
)

// UpgradeStartResponse POST /api/system/upgrade 响应。
type UpgradeStartResponse struct {
	State       UpgradeState `json:"state"`
	RequestedAt string       `json:"requestedAt"`
}

// UpgradeStatusResponse GET /api/system/upgrade 响应。
type UpgradeStatusResponse struct {
	State      UpgradeState `json:"state"`
	StartedAt  string       `json:"startedAt,omitempty"`
	FinishedAt string       `json:"finishedAt,omitempty"`
	ExitCode   *int         `json:"exitCode,omitempty"`
	Output     string       `json:"output,omitempty"`
}
