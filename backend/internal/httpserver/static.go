package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// staticHandler 提供前端静态文件服务和 Vue history 路由回退。
// API 请求（/api/）不经过此 handler，由调用方在路由层分流。
//
// 缓存：index.html 每次都要向服务器确认（更新版本后浏览器马上拿到新页面）；/assets/ 下的文件名带内容哈希，
// 可以长期缓存，不存在时直接 404（不回退到 index.html，否则旧页面引用的脚本会被当成 HTML 加载，页面空白）。
func staticHandler(publicDir string) http.Handler {
	fs := http.Dir(publicDir)
	fileServer := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 安全：禁止路径穿越
		cleanPath := filepath.Clean(r.URL.Path)
		if strings.Contains(cleanPath, "..") {
			http.NotFound(w, r)
			return
		}
		hashedAsset := strings.HasPrefix(filepath.ToSlash(cleanPath), "/assets/")

		// 检查文件是否存在
		fullPath := filepath.Join(publicDir, filepath.FromSlash(cleanPath))
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			// 文件存在，直接返回
			switch {
			case hashedAsset:
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			case filepath.Base(cleanPath) == "index.html":
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if hashedAsset {
			http.NotFound(w, r)
			return
		}

		// 文件不存在或是目录，回退到 index.html（Vue history 路由）
		indexPath := filepath.Join(publicDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
	})
}
