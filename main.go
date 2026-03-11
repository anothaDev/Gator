package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/handlers"
	"github.com/raul/gator/internal/routes"
	"github.com/raul/gator/internal/storage"
)

func main() {
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "gator.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatal("Failed to create database directory:", err)
	}

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize sqlite store:", err)
	}
	defer store.Close()

	r := gin.Default()
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})

	setupHandler := handlers.NewSetupHandler(store)
	opnsenseHandler := handlers.NewOPNsenseHandler(store)
	vpnHandler := handlers.NewVPNHandler(store)
	gatewayHandler := handlers.NewGatewayHandler(store)
	appRoutingHandler := handlers.NewAppRoutingHandler(store)
	ipRangesHandler := handlers.NewIPRangesHandler(store)
	tunnelHandler := handlers.NewTunnelHandler(store)

	// Serve the SolidJS frontend static files
	r.Static("/assets", "./frontend/dist/assets")
	r.StaticFile("/", "./frontend/dist/index.html")

	// Catch-all: return JSON 404 for API paths, serve index.html for SPA client-side routing.
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.File("./frontend/dist/index.html")
	})

	// Register API routes
	routes.Register(r, setupHandler, opnsenseHandler, vpnHandler, gatewayHandler, appRoutingHandler, ipRangesHandler, tunnelHandler)

	// Start background jobs.
	handlers.StartASNRefreshLoop(store)
	handlers.StartPendingRevisionCleanup(store)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s (db: %s)", port, dbPath)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
