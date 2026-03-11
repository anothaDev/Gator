package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/handlers"
)

func registerAppRoutingRoutes(api *gin.RouterGroup, appRouting *handlers.AppRoutingHandler) {
	api.GET("/app-profiles", appRouting.ListProfiles)
	api.POST("/app-profiles", appRouting.CreateCustomProfile)
	api.DELETE("/app-profiles/:profileId", appRouting.DeleteCustomProfile)
}
