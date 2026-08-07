# 实施计划

1. 在 `backend/internal/httpserver/protected_path_test.go` 增加用户接口鉴权和 CORS 回归测试，先验证旧实现失败。
2. 在 `backend/internal/modules/auth/handler_test.go` 增加废弃公开认证路由不存在、真实登录路由仍存在的测试，先验证旧实现失败。
3. 修改 `backend/internal/httpserver/server.go`：把 `/api/users` 纳入鉴权；CORS 只信任明确配置的来源。
4. 修改认证模块和配置：移除注册、验证码、占位密码登录、占位 API Key 登录的路由与死代码，保留管理员初始化和真实登录。
5. 删除前端不可达注册页及相关 API、类型和文案。
6. 运行认证与 HTTP server 定向测试并检查测试残留。
7. 按项目固定服务规则重启前后端，验证未登录用户接口、CORS、已删除路由和健康检查。
8. 运行受控全量测试和二次 review，把结果写入 `04-VERIFICATION.md`。
