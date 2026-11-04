package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"transithub/backend/internal/config"
	"transithub/backend/internal/database"
	"transithub/backend/internal/database/migrations"
	"transithub/backend/internal/httpserver"
	"transithub/backend/internal/selfupdate"
)

func main() {
	cfg := config.Load()
	// 在线更新：装了比自己新的版本就切换过去（成功时不会返回）；上次切换后没启动成功的版本不再使用。
	selfupdate.Launch(cfg.DataDir, cfg.AppVersion)
	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	// 数据库迁移：连接 DB 后立即执行，保证表结构就绪后再初始化业务模块
	if err := migrations.Run(ctx, db); err != nil {
		log.Fatalf("[migrations] %v", err)
	}

	// Redis 用于仪表盘 admin 会话存储与令牌自动刷新调度。
	redisClient, err := database.ConnectRedis(ctx, cfg.RedisURL)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer redisClient.Close()

	server := httpserver.New(cfg, db, redisClient)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	// 迁移已完成、端口已监听：如果这是刚在线更新上来的版本，确认启动成功。
	selfupdate.MarkActive(cfg.DataDir, cfg.AppVersion)

	go func() {
		log.Printf("backend listening on :%s", cfg.Port)
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server background shutdown: %v", err)
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
