package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/handlers"
)

func registerIPRangesRoutes(api *gin.RouterGroup, h *handlers.IPRangesHandler) {
	g := api.Group("/ip-ranges")
	g.GET("", h.List)
	g.POST("/upload", h.Upload)
	g.GET("/serve/:filename", h.Serve)
	g.DELETE("/:filename", h.Delete)
}
