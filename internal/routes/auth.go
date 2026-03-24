package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/anothaDev/gator/internal/handlers"
)

func registerAuthRoutes(api *gin.RouterGroup, auth *handlers.AuthHandler) {
	authGroup := api.Group("/auth")
	{
		authGroup.GET("/status", auth.Status)
		authGroup.POST("/bootstrap", auth.Bootstrap)
		authGroup.POST("/login", auth.Login)
		authGroup.POST("/logout", auth.Logout)
	}
}
