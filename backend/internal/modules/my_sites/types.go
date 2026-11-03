package my_sites

import "transithub/backend/internal/modules/upstream"

// 错误标识只在后端内部流转：调用方（仪表盘、用户活跃、群发邮件）各自映射成自己的 i18n key，
// user_activity 依赖其中的 "authRequired" / "adminOnly" 子串判断是否需要重新登录，修改时需同步。
const (
	ErrorAuthRequired = "admin.mySites.errors.authRequired"
	ErrorAdminOnly    = "admin.mySites.errors.adminOnly"
)

// State 是一个工作区「我的站点」（自己的 sub2api / new-api）的管理员会话，存储于 my_site_states 表。
type State struct {
	UserID         string
	AdminAccountID string
	BaseURL        string
	Email          string
	Session        upstream.Session
}

type requestError string

func (e requestError) Error() string { return string(e) }
