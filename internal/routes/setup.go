package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/handlers"
)

func registerSetupRoutes(api *gin.RouterGroup, setup *handlers.SetupHandler) {
	setupGroup := api.Group("/setup")
	{
		setupGroup.GET("/status", setup.GetStatus)
		setupGroup.POST("/save", setup.SaveConfig)

		// Backward-compatible endpoint. Prefer provider-specific routes.
		setupGroup.POST("/test", setup.TestConnection)
	}

	// Instance management
	instances := api.Group("/instances")
	{
		instances.GET("", setup.ListInstances)
		instances.POST("/:id/activate", setup.SwitchInstance)
		instances.DELETE("/:id", setup.DeleteInstanceHandler)
	}
}
