package vmctl

import (
	"fmt"
	"io/fs"
	"os"

	agentvm "github.com/vrealzhou/agent-vm"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func getPort(flagPort string) string {
	if flagPort != "" {
		return flagPort
	}
	if envPort := os.Getenv("VM_MANAGER_PORT"); envPort != "" {
		return envPort
	}
	return "8080"
}

func LaunchWebServer(flagPort string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	port := getPort(flagPort)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLogger())

	// Static files from embedded FS
	staticFS, err := fs.Sub(agentvm.WebStatic, "web/static")
	if err != nil {
		return err
	}
	e.StaticFS("/static", staticFS)

	// Main page
	e.GET("/", func(c *echo.Context) error {
		index, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			return c.String(500, "index.html not found")
		}
		return c.HTMLBlob(200, index)
	})

	// API routes
	registerAPIRoutes(e, cfg)

	fmt.Printf("Web UI running at http://localhost:%s\n", port)
	return e.Start(":" + port)
}

func registerAPIRoutes(e *echo.Echo, cfg Config) {
	// VM control
	e.GET("/api/status", handleStatus(cfg))
	e.POST("/api/bootstrap", handleBootstrap(cfg))
	e.POST("/api/start", handleStart(cfg))
	e.POST("/api/stop", handleStop(cfg))
	e.POST("/api/destroy", handleDestroy(cfg))
	e.POST("/api/upgrade-kernel", handleUpgradeKernel(cfg))

	// Tunnels
	e.GET("/api/tunnels", handleListTunnels(cfg))
	e.POST("/api/tunnels", handleAddTunnel(cfg))
	e.DELETE("/api/tunnels/:id", handleRemoveTunnel(cfg))
	e.POST("/api/tunnels/:id/start", handleStartTunnel(cfg))
	e.POST("/api/tunnels/:id/stop", handleStopTunnel(cfg))

	// Sync
	e.GET("/api/sync", handleListSync(cfg))
	e.POST("/api/sync", handleAddSync(cfg))
	e.DELETE("/api/sync/:id", handleRemoveSync(cfg))
	e.POST("/api/sync/:id/run", handleRunSync(cfg))
	e.GET("/api/sync/:id/history", handleSyncHistory(cfg))

	// VM Files
	e.GET("/api/vm-files", handleVMFiles(cfg))

	// Host Files
	e.GET("/api/host-files", handleHostFiles())

	// Progress
	e.GET("/api/progress", handleProgress())
}
