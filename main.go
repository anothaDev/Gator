package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anothaDev/gator/internal/handlers"
	"github.com/anothaDev/gator/internal/routes"
	"github.com/anothaDev/gator/internal/storage"
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

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/opnsense/overview/stream"},
	}))
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})

	setupHandler := handlers.NewSetupHandler(store)
	authHandler := handlers.NewAuthHandler(store)
	opnsenseHandler := handlers.NewOPNsenseHandler(store)
	tailscaleHandler := handlers.NewTailscaleHandler(store)
	vpnHandler := handlers.NewVPNHandler(store)
	gatewayHandler := handlers.NewGatewayHandler(store)
	appRoutingHandler := handlers.NewAppRoutingHandler(store)
	ipRangesHandler := handlers.NewIPRangesHandler(store)
	tunnelHandler := handlers.NewTunnelHandler(store)
	r.Use(authHandler.Middleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Register API routes
	routes.Register(r, authHandler, setupHandler, opnsenseHandler, tailscaleHandler, vpnHandler, gatewayHandler, appRoutingHandler, ipRangesHandler, tunnelHandler)
	serveFrontend(r)

	// Start background jobs.
	reconciler := handlers.NewReconciler(store, 60*time.Second)
	vpnHandler.SetReconciler(reconciler)
	tunnelHandler.SetReconciler(reconciler)
	reconciler.Start()
	defer reconciler.Stop()
	defer opnsenseHandler.Stop()

	stopASN := handlers.StartASNRefreshLoop(store)
	defer stopASN()

	stopRevisionCleanup := handlers.StartPendingRevisionCleanup(store)
	defer stopRevisionCleanup()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on :%s (db: %s)", port, dbPath)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
