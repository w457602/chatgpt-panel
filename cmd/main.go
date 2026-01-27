package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/webauto/chatgpt-panel/internal/config"
	"github.com/webauto/chatgpt-panel/internal/handlers"
	"github.com/webauto/chatgpt-panel/internal/middleware"
	"github.com/webauto/chatgpt-panel/internal/models"
	"github.com/webauto/chatgpt-panel/internal/services"
)

func main() {
	// 加载配置
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化数据库
	if err := models.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// 设置Gin模式
	gin.SetMode(config.AppConfig.GinMode)

	// 创建路由
	r := gin.Default()

	// 中间件
	r.Use(middleware.CORSMiddleware())

	// 静态文件
	r.Static("/static", "./static")
	r.LoadHTMLGlob("templates/*")

	// 首页
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	// 初始化处理器
	authHandler := handlers.NewAuthHandler()
	accountHandler := handlers.NewAccountHandler()
	oauthHandler := handlers.NewOAuthHandler()

	// 启动 Codex OAuth 回调服务
	if err := services.GetCodexOAuthService().EnsureCallbackServer(); err != nil {
		log.Printf("Codex OAuth callback server failed to start: %v", err)
	}

	// API路由
	api := r.Group("/api/v1")
	{
		// 认证（无需登录）
		api.POST("/auth/login", authHandler.Login)

		// 账号导入（API Key认证）
		api.POST("/accounts/import", accountHandler.Import)

		// 需要登录的路由
		auth := api.Group("")
		auth.Use(middleware.AuthMiddleware())
		{
			// 用户信息
			auth.GET("/auth/me", authHandler.GetCurrentUser)
			auth.POST("/auth/change-password", authHandler.ChangePassword)

			// 账号管理
			auth.GET("/accounts", accountHandler.List)
			auth.GET("/accounts/stats", accountHandler.GetStats)
			auth.GET("/accounts/:id", accountHandler.Get)
			auth.POST("/accounts", accountHandler.Create)
			auth.PUT("/accounts/:id", accountHandler.Update)
			auth.DELETE("/accounts/:id", accountHandler.Delete)
			auth.POST("/accounts/batch-delete", accountHandler.BatchDelete)
			auth.PATCH("/accounts/:id/status", accountHandler.UpdateStatus)
			auth.POST("/accounts/batch-status", accountHandler.BatchUpdateStatus)
			auth.PATCH("/accounts/:id/refresh-token", accountHandler.UpdateRefreshToken)

			// 账号测试
			auth.POST("/accounts/:id/test", accountHandler.TestAccount)
			auth.POST("/accounts/batch-test", accountHandler.BatchTestAccounts)
			auth.GET("/accounts/batch-test/:task_id", accountHandler.GetBatchTestResult)
			auth.POST("/accounts/:id/refresh", accountHandler.RefreshAccountToken)

			// Codex OAuth (PKCE)
			auth.POST("/oauth/codex/start", oauthHandler.StartCodex)
			auth.GET("/oauth/codex/status/:state", oauthHandler.GetStatus)
			auth.POST("/oauth/codex/callback", oauthHandler.SubmitCallback)
		}
	}

	// 启动服务
	log.Printf("🚀 ChatGPT Panel starting on port %s", config.AppConfig.ServerPort)
	if err := r.Run(":" + config.AppConfig.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
