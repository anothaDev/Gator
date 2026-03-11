package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/handlers"
)

func registerVPNRoutes(api *gin.RouterGroup, vpn *handlers.VPNHandler) {
	vpnGroup := api.Group("/vpn")
	{
		vpnGroup.GET("/configs", vpn.ListConfigs)
		vpnGroup.POST("/configs", vpn.CreateConfig)
		vpnGroup.GET("/configs/:id", vpn.GetConfig)
		vpnGroup.PUT("/configs/:id", vpn.SaveConfig)
		vpnGroup.DELETE("/configs/:id", vpn.DeleteConfig)
	}
}
