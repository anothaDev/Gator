package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/models"
)

const backupDir = "data/backups"

// createInitialBackup downloads the OPNsense config if no backups exist yet.
// Runs in a background goroutine — errors are logged, not returned.
func createInitialBackup(cfg models.FirewallConfig) {
	// Check if any backup already exists.
	entries, err := os.ReadDir(backupDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".xml") {
				return // Already have a backup.
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	api := newOPNsenseAPIClient(cfg)
	raw, err := api.GetRaw(ctx, "/api/core/backup/download/this")
	if err != nil {
		log.Printf("[backup] initial backup failed: %v", err)
		return
	}
	if len(raw) < 100 {
		log.Printf("[backup] initial backup too small (%d bytes), skipping", len(raw))
		return
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		log.Printf("[backup] failed to create backup dir: %v", err)
		return
	}

	filename := fmt.Sprintf("opnsense-initial-%s.xml", time.Now().UTC().Format("20060102-150405"))
	if err := os.WriteFile(filepath.Join(backupDir, filename), raw, 0o600); err != nil {
		log.Printf("[backup] failed to write initial backup: %v", err)
		return
	}

	log.Printf("[backup] initial config snapshot saved: %s (%d bytes)", filename, len(raw))
}

// ListBackups returns stored OPNsense config backups.
func (h *GatewayHandler) ListBackups(c *gin.Context) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"backups": []any{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read backup directory"})
		return
	}

	type backupInfo struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Created  string `json:"created"`
	}

	backups := make([]backupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupInfo{
			Filename: entry.Name(),
			Size:     info.Size(),
			Created:  info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	// Sort newest first.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Created > backups[j].Created
	})

	c.JSON(http.StatusOK, gin.H{"backups": backups})
}

// CreateBackup downloads the current OPNsense config and stores it locally.
func (h *GatewayHandler) CreateBackup(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	raw, err := api.GetRaw(ctx, "/api/core/backup/download/this")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to download config: " + err.Error()})
		return
	}

	if len(raw) < 100 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "received suspiciously small config backup"})
		return
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backup directory"})
		return
	}

	filename := fmt.Sprintf("opnsense-%s.xml", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(backupDir, filename)

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write backup file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "created",
		"filename": filename,
		"size":     len(raw),
	})
}

// DownloadBackup serves a stored backup file.
func (h *GatewayHandler) DownloadBackup(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	path := filepath.Join(backupDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	c.File(path)
}

// DeleteBackup removes a stored backup file.
func (h *GatewayHandler) DeleteBackup(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	path := filepath.Join(backupDir, filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete backup"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "filename": filename})
}
