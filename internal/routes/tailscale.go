package routes

import (
	"github.com/anothaDev/gator/internal/handlers"
	"github.com/gin-gonic/gin"
)

func registerTailscaleRoutes(opnsense *gin.RouterGroup, tailscale *handlers.TailscaleHandler) {
	tg := opnsense.Group("/tailscale")
	{
		tg.GET("/status", tailscale.Status)
		tg.POST("/install", tailscale.Install)
		tg.GET("/install-status", tailscale.InstallStatus)
		tg.POST("/configure", tailscale.Configure)
		tg.GET("/subnets", tailscale.ListSubnets)
		tg.POST("/subnets", tailscale.AddSubnet)
		tg.DELETE("/subnets/:uuid", tailscale.DeleteSubnet)
	}
}
