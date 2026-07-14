package system

// VersionResponse GET /api/system/version 响应。
// 开源版只展示版本号，不再包含授权状态或更新开关字段。
type VersionResponse struct {
	Version string `json:"version"`
}
