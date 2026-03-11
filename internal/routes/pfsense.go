package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/handlers"
)

func registerPfSenseRoutes(api *gin.RouterGroup, setup *handlers.SetupHandler) {
	pfsense := api.Group("/pfsense")
	{
		pfsense.POST("/test-connection", setup.TestPfSenseConnection)
	}
}
