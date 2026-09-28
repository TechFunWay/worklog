package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"smallgo/server/apps"
	"smallgo/server/audit"
	"smallgo/server/config"
	"smallgo/server/database"
	"smallgo/server/logger"
	"smallgo/server/middleware"
	"smallgo/server/realtime"
	"smallgo/server/response"
	"smallgo/server/scheduler"
	"smallgo/server/security"
	"smallgo/server/stats"
	"smallgo/server/sysconfig"
	"smallgo/server/upload"
	"smallgo/server/user"
	"smallgo/server/version"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Start() {
	cfg := config.C

	if cfg.ShowHelp {
		config.PrintHelp()
		os.Exit(0)
	}

	if cfg.ShowVersion {
		version.PrintVersion()
		os.Exit(0)
	}

	if cfg.ResetAdminPassword {
		db, err := database.InitDB(cfg.DBPath)
		if err != nil {
			fmt.Printf("连接数据库失败: %v\n", err)
			os.Exit(1)
		}
		defer database.CloseDB(db)

		if err := user.ResetAdminPassword(db); err != nil {
			fmt.Printf("重置管理员密码失败: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if cfg.LogMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := logger.Init(cfg.LogDir, cfg.LogRetentionDays, cfg.LogConsole); err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		fmt.Printf("连接数据库失败: %v\n", err)
		os.Exit(1)
	}
	defer database.CloseDB(db)

	if err := database.AutoMigrate(db); err != nil {
		fmt.Printf("数据库结构迁移失败: %v\n", err)
		os.Exit(1)
	}

	if err := database.RunUpgrades(db, version.Version, database.Upgrades); err != nil {
		fmt.Printf("执行数据升级失败: %v\n", err)
		os.Exit(1)
	}

	if err := apps.MigrateAll(db); err != nil {
		fmt.Printf("应用数据迁移失败: %v\n", err)
		os.Exit(1)
	}

	if err := sysconfig.InitDefaultConfigs(db); err != nil {
		fmt.Printf("初始化默认配置失败: %v\n", err)
		os.Exit(1)
	}

	logger.SetRetentionFunc(func() int {
		val, err := sysconfig.GetConfig(db, "log_retention_days", 0)
		if err != nil || val == "" {
			return cfg.LogRetentionDays
		}
		if days, err := strconv.Atoi(val); err == nil {
			return days
		}
		return cfg.LogRetentionDays
	})
	// Keep database-backed operation history aligned with the administrator's
	// log retention policy. The job is registered before scheduler.Start.
	audit.RegisterCleanup(db)

	jwtSecret, err := sysconfig.GetConfig(db, "jwt_secret", 0)
	if err != nil || jwtSecret == "" {
		fmt.Println("读取 JWT 密钥失败")
		os.Exit(1)
	}

	r := NewRouter(cfg, db, jwtSecret)

	// 匿名使用统计心跳：-disable-stats / DISABLE_STATS=1 可完全退出，
	// 运行时也可由管理员通过 stats_enabled 配置关闭。
	if !cfg.DisableStats {
		deviceType := "docker"
		if cfg.FnOSApp {
			deviceType = "fnos"
		}
		stats.Start(version.AppID, version.Version, deviceType, cfg.DataDir)
		defer stats.Stop()
	}

	// Start background jobs registered by the framework or apps.
	sched := scheduler.Start()
	defer sched.Stop()

	logger.Info("========================================")
	logger.Info("  %s %s", version.AppName, version.Version)
	logger.Info("========================================")
	logger.Info("  Data Dir:    %s", cfg.DataDir)
	logger.Info("  Database:    %s", cfg.DBPath)
	logger.Info("  Web Dir:     %s", cfg.WebDir)
	logger.Info("  Upload Dir:  %s", cfg.UploadDir)
	if cfg.FnOSApp {
		logger.Info("  Gateway:     %s", cfg.GatewayPrefix)
		logger.Info("  Socket:      %s", cfg.GatewaySocket)
		logger.Info("  Access URL:  http://localhost:%d", cfg.Port)
	} else {
		logger.Info("  Access URL:  http://localhost:%d", cfg.Port)
	}
	if cfg.RateLimit > 0 {
		logger.Info("  Rate Limit:  %d req/min per IP", cfg.RateLimit)
	}
	logger.Info("========================================")

	listeners, err := listen(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听端口 %d 失败: %v\n", cfg.Port, err)
		logger.Error("Failed to listen: %v", err)
		os.Exit(1)
	}
	for _, listener := range listeners {
		defer listener.Listener.Close()
	}
	if cfg.FnOSApp {
		defer os.Remove(cfg.GatewaySocket)
	}

	servers := make([]*http.Server, 0, len(listeners))
	for _, listener := range listeners {
		handler := http.Handler(r)
		if !listener.IsFnOSGateway {
			handler = newDirectHandler(r, cfg)
		}
		srv := newHTTPServer(handler, listener.IsFnOSGateway)
		servers = append(servers, srv)
		go func(srv *http.Server, listener net.Listener) {
			if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "服务运行出错: %v\n", err)
				logger.Error("Server error: %v", err)
				os.Exit(1)
			}
		}(srv, listener.Listener)
	}

	printStartupBanner(cfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("Server forced to shutdown: %v", err)
		}
	}

	apps.ShutdownAll()

	logger.Info("Server exited")
}

// printStartupBanner 在端口监听成功后往终端打一段固定的简洁中文信息。
// 运行期日志（请求、统计、后台任务）一律只写文件，终端不再输出别的内容，
// 让 make dev / 前台运行时始终能看到访问地址又不被日志刷屏。
func printStartupBanner(cfg config.Config) {
	fmt.Println()
	fmt.Printf("  工记 WorkLog %s 已启动\n", version.Version)
	if cfg.FnOSApp {
		fmt.Printf("  网关路径:  %s\n", cfg.GatewayPrefix)
		fmt.Printf("  直连地址:  http://localhost:%d（访问后自动跳转网关）\n", cfg.Port)
	} else {
		fmt.Printf("  访问地址:  http://localhost:%d\n", cfg.Port)
	}
	fmt.Printf("  数据目录:  %s\n", cfg.DataDir)
	fmt.Printf("  文件日志:  %s\n", cfg.LogDir)
	fmt.Println("  停止服务:  按 Ctrl+C")
	fmt.Println()
}

// newDirectHandler keeps the configured TCP port useful in fnOS mode while
// preserving the single canonical gateway-prefixed SPA URL. The browser is
// redirected before loading assets, so Vite's /app/<appname>/ base is intact.
func newDirectHandler(handler http.Handler, cfg config.Config) http.Handler {
	if !cfg.FnOSApp {
		return handler
	}
	prefix := strings.TrimSuffix(cfg.GatewayPrefix, "/") + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, prefix, http.StatusTemporaryRedirect)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

type listenerBinding struct {
	Listener      net.Listener
	IsFnOSGateway bool
}

func newHTTPServer(handler http.Handler, fnOSGateway bool) *http.Server {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if fnOSGateway {
		srv.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
			return middleware.MarkFnOSGateway(ctx)
		}
	}
	return srv
}

func listen(cfg config.Config) ([]listenerBinding, error) {
	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return nil, err
	}
	listeners := []listenerBinding{{Listener: tcpListener}}
	if !cfg.FnOSApp {
		return listeners, nil
	}
	if cfg.GatewaySocket == "" {
		tcpListener.Close()
		return nil, fmt.Errorf("-fnos-app requires -gateway-socket")
	}
	if !strings.HasPrefix(cfg.GatewayPrefix, "/app/") {
		tcpListener.Close()
		return nil, fmt.Errorf("-gateway-prefix must begin with /app/")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.GatewaySocket), 0755); err != nil {
		tcpListener.Close()
		return nil, fmt.Errorf("create gateway socket directory: %w", err)
	}
	if err := os.Remove(cfg.GatewaySocket); err != nil && !os.IsNotExist(err) {
		tcpListener.Close()
		return nil, fmt.Errorf("remove stale gateway socket: %w", err)
	}
	listener, err := net.Listen("unix", cfg.GatewaySocket)
	if err != nil {
		tcpListener.Close()
		return nil, fmt.Errorf("listen on gateway socket: %w", err)
	}
	return append(listeners, listenerBinding{Listener: listener, IsFnOSGateway: true}), nil
}

// NewRouter builds the full API router (all middleware, routes and registered
// apps) without starting an HTTP listener. Start uses it in production; tests
// use it to exercise the API against an in-memory database.
func NewRouter(cfg config.Config, db *gorm.DB, jwtSecret string) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.CORS(cfg.CORSOrigin))
	r.Use(middleware.LimitJSONBody(1 << 20))

	appGroup := r.Group("")
	if cfg.FnOSApp {
		appGroup = r.Group(strings.TrimSuffix(cfg.GatewayPrefix, "/"))
	}
	api := appGroup.Group("/api")

	api.GET("/version", func(c *gin.Context) {
		response.Success(c, version.GetVersion())
	})

	// Liveness/readiness probe for Docker, NAS health checks, uptime monitors.
	api.GET("/health", func(c *gin.Context) {
		response.Success(c, gin.H{
			"status":  "ok",
			"version": version.Version,
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// Rate limiter for public endpoints
	var publicLimiter gin.HandlerFunc
	if cfg.RateLimit > 0 {
		rl := middleware.NewRateLimiter(cfg.RateLimit, 1*time.Minute)
		publicLimiter = middleware.LimitRequests(rl)
	} else {
		publicLimiter = middleware.LimitRequests(nil)
	}

	// Auth route groups shared by all features.
	publicGroup := api.Group("", publicLimiter)
	optionalAuthGroup := api.Group("")
	optionalAuthGroup.Use(middleware.OptionalAuth(jwtSecret, db))
	authGroup := api.Group("")
	authGroup.Use(middleware.RequireAuth(jwtSecret, db))
	authGroup.Use(audit.MutationLogger(db))
	adminGroup := api.Group("")
	adminGroup.Use(middleware.RequireAuth(jwtSecret, db))
	adminGroup.Use(middleware.RequireAdmin())
	adminGroup.Use(audit.MutationLogger(db))

	// Config: public read is rate-limited; user config read & write require
	// auth; system config read & metadata require admin.
	sysconfig.RegisterRoutes(publicGroup, authGroup, adminGroup, db)

	// Security questions: forgot-password flow is public (rate limited); managing
	// one's own questions requires auth.
	security.RegisterRoutes(publicGroup, authGroup, db)

	// File upload requires authentication; uploaded content is still readable by
	// URL so it can be embedded in public pages.
	authGroup.POST("/upload", upload.HandleUpload(cfg.UploadDir))
	appGroup.GET("/uploads/*filepath", upload.ServeUpload(cfg.UploadDir))

	user.RegisterRoutes(publicGroup, optionalAuthGroup, authGroup, adminGroup, db, cfg.FnOSApp)

	// 复用匿名统计通道发送一次 donate_support 事件。
	stats.RegisterRoutes(authGroup, db)

	// Server-Sent Events stream for the default realtime broker. Apps push
	// live updates via realtime.Publish; browsers subscribe at /api/realtime.
	authGroup.GET("/realtime", realtime.Handler())

	// Admin operation log (browse + CSV export).
	audit.RegisterRoutes(adminGroup, db)

	// Admin upgrade history: every applied one-off data migration, newest first.
	adminGroup.GET("/upgrade-history", func(c *gin.Context) {
		records, err := database.ListUpgradeRecords(db)
		if err != nil {
			response.ErrorInternal(c, "读取升级历史失败")
			return
		}
		response.Success(c, records)
	})

	// Register apps from the registry (includes the qrcode example app).
	apps.SetupAll(publicGroup, db)
	apps.SetupProtectedAll(authGroup, adminGroup, db)

	if cfg.WebDir != "" {
		appGroup.Static("/assets", cfg.WebDir+"/assets")
		appGroup.StaticFile("/favicon.ico", cfg.WebDir+"/favicon.ico")
		appGroup.StaticFile("/favicon.svg", cfg.WebDir+"/favicon.svg")
		appGroup.StaticFile("/favicon-16.png", cfg.WebDir+"/favicon-16.png")
		appGroup.StaticFile("/favicon-32.png", cfg.WebDir+"/favicon-32.png")
		appGroup.StaticFile("/favicon-192.png", cfg.WebDir+"/favicon-192.png")
		appGroup.StaticFile("/favicon-512.png", cfg.WebDir+"/favicon-512.png")
		appGroup.StaticFile("/apple-touch-icon.png", cfg.WebDir+"/apple-touch-icon.png")
		appGroup.GET("/manifest.webmanifest", func(c *gin.Context) {
			c.Header("Content-Type", "application/manifest+json")
			c.File(cfg.WebDir + "/manifest.webmanifest")
		})
		r.NoRoute(func(c *gin.Context) {
			if !cfg.FnOSApp || c.Request.URL.Path == strings.TrimSuffix(cfg.GatewayPrefix, "/") || strings.HasPrefix(c.Request.URL.Path, strings.TrimSuffix(cfg.GatewayPrefix, "/")+"/") {
				serveSPA(c, cfg)
				return
			}
			c.Status(http.StatusNotFound)
		})
	}

	return r
}

// serveSPA 返回 SPA 入口 HTML，并把 __FNOS_GATEWAY_FLAG__ 替换成「这次文档
// 请求是不是从飞牛网关 socket 进来的」：只有服务端知道答案（两条监听器分别
// 是网关 socket 与直连端口），而前端必须知道——网关域上不能带应用自己的
// Authorization，会被接入层当成无效会话 token 直接拒掉（200 "invalid token"，
// 请求到不了应用），见 web/src/utils/gateway.ts。占位符只出现在字符串字面量里
// （window.__FNOS_GATEWAY__ = "__FNOS_GATEWAY_FLAG__" === "true"），
// 这样开发服务器不替换时它就是 false，也不会误伤属性名。
func serveSPA(c *gin.Context, cfg config.Config) {
	data, err := os.ReadFile(filepath.Join(cfg.WebDir, "index.html"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	page := strings.ReplaceAll(string(data), "__FNOS_GATEWAY_FLAG__", strconv.FormatBool(middleware.OnFnOSGateway(c.Request.Context())))
	// index.html must revalidate on every load: hashed asset filenames handle
	// long-term caching, so the entry must never be cached or phones keep
	// running a stale bundle after an upgrade.
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}
