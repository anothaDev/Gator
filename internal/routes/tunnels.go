package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/handlers"
)

func registerTunnelRoutes(api *gin.RouterGroup, tunnels *handlers.TunnelHandler) {
	g := api.Group("/tunnels")
	{
		g.GET("", tunnels.ListTunnels)
		g.POST("", tunnels.CreateTunnel)
		g.GET("/discover", tunnels.DiscoverTunnels)
		g.POST("/import", tunnels.ImportTunnel)
		g.GET("/next-subnet", tunnels.NextSubnet)
		g.POST("/test-ssh", tunnels.TestSSH)
		g.GET("/:id", tunnels.GetTunnel)
		g.PUT("/:id", tunnels.SaveTunnel)
		g.DELETE("/:id", tunnels.DeleteTunnel)
		g.POST("/:id/deploy", tunnels.DeployStep)
		g.GET("/:id/status", tunnels.TunnelStatus)
		g.POST("/:id/teardown", tunnels.TeardownTunnel)
		g.POST("/:id/restart", tunnels.RestartTunnel)
		g.POST("/:id/cross-check", tunnels.CrossCheck)
		g.POST("/:id/lockdown-ssh", tunnels.LockdownSSH)
	}
}
