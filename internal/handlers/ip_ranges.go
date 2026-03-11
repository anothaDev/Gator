package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/models"
)

const ipRangesDir = "data/ip-ranges"

// IPRangesStore is the storage interface needed by IPRangesHandler.
type IPRangesStore interface {
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
}

// IPRangesHandler manages IP range file uploads and serving.
type IPRangesHandler struct {
	store IPRangesStore
}

// NewIPRangesHandler creates a new handler.
func NewIPRangesHandler(store IPRangesStore) *IPRangesHandler {
	return &IPRangesHandler{store: store}
}

// Upload handles file upload for IP range JSON files.
// POST /api/ip-ranges/upload
// Expects multipart form with "file" field and optional "filename" field.
func (h *IPRangesHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file upload required"})
		return
	}
	defer file.Close()

	// Determine filename: use the provided "filename" form field if set, otherwise the upload name.
	filename := c.PostForm("filename")
	if filename == "" {
		filename = header.Filename
	}
	// Sanitize: only allow alphanumeric, underscore, hyphen, dot.
	filename = sanitizeFilename(filename)
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	// Ensure directory exists.
	if err := os.MkdirAll(ipRangesDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create ip-ranges directory"})
		return
	}

	destPath := filepath.Join(ipRangesDir, filename)
	dst, err := os.Create(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file: " + err.Error()})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write file: " + err.Error()})
		return
	}

	log.Printf("[IPRanges] uploaded %s (%d bytes)", filename, written)
	c.JSON(http.StatusOK, gin.H{
		"status":   "uploaded",
		"filename": filename,
		"size":     written,
	})
}

// Serve returns an IP range JSON file for OPNsense to fetch as a URL table.
// GET /api/ip-ranges/serve/:filename
func (h *IPRangesHandler) Serve(c *gin.Context) {
	filename := sanitizeFilename(c.Param("filename"))
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	filePath := filepath.Join(ipRangesDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found: " + filename})
		return
	}

	c.Header("Content-Type", "application/json")
	c.File(filePath)
}

// List returns all uploaded IP range files.
// GET /api/ip-ranges
func (h *IPRangesHandler) List(c *gin.Context) {
	entries, err := os.ReadDir(ipRangesDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"files": []string{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list files"})
		return
	}

	type fileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	files := make([]fileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{Name: e.Name(), Size: info.Size()})
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// Delete removes an uploaded IP range file.
// DELETE /api/ip-ranges/:filename
func (h *IPRangesHandler) Delete(c *gin.Context) {
	filename := sanitizeFilename(c.Param("filename"))
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	filePath := filepath.Join(ipRangesDir, filename)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "filename": filename})
}

// sanitizeFilename strips anything unsafe from a filename.
func sanitizeFilename(name string) string {
	// Remove path separators.
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

// ipRangesFileExists checks if an IP range file has been uploaded.
func ipRangesFileExists(filename string) bool {
	filePath := filepath.Join(ipRangesDir, filename)
	_, err := os.Stat(filePath)
	return err == nil
}

// ensureURLTableAlias creates or updates an OPNsense URL Table (JSON) alias
// that points to a Gator-served IP ranges file.
// gatorBaseURL is the URL that OPNsense can reach Gator at (e.g. "http://192.168.1.100:8080").
func ensureURLTableAlias(
	ctx context.Context,
	api *opnsenseAPIClient,
	aliasName string,
	gatorBaseURL string,
	filename string,
	jqFilter string,
) (string, bool, error) {
	serveURL := fmt.Sprintf("%s/api/ip-ranges/serve/%s", gatorBaseURL, filename)

	payload := map[string]any{
		"alias": map[string]any{
			"enabled":     "1",
			"name":        aliasName,
			"type":        "urltable",
			"proto":       "IPv4",
			"content":     serveURL,
			"description": "Gator IP-based routing (URL table, auto-managed)",
		},
	}

	// If we have a jq filter, use the JSON URL table type instead.
	if jqFilter != "" {
		payload = map[string]any{
			"alias": map[string]any{
				"enabled":             "1",
				"name":                aliasName,
				"type":                "urltable",
				"proto":               "IPv4",
				"content":             serveURL,
				"url_content_type":    "json",
				"url_path_expression": jqFilter,
				"updatefreq":          "0.5", // refresh every 12 hours (0.5 days)
				"description":         "Gator IP-based routing (URL table JSON, auto-managed)",
			},
		}
	}

	// Search for existing alias by name.
	searchResp, err := api.Post(ctx, "/api/firewall/alias/search_item", map[string]any{})
	if err == nil {
		for _, raw := range asSlice(searchResp["rows"]) {
			row := asMap(raw)
			if asString(row["name"]) == aliasName {
				uuid := asString(row["uuid"])
				if uuid != "" {
					_, err := api.Post(ctx, "/api/firewall/alias/set_item/"+uuid, payload)
					if err != nil {
						return "", false, fmt.Errorf("update URL table alias %s: %w", aliasName, err)
					}
					log.Printf("[Alias] updated URL table alias %s (%s) -> %s", aliasName, uuid, serveURL)
					return uuid, false, nil
				}
			}
		}
	}

	// Create new alias.
	resp, err := api.Post(ctx, "/api/firewall/alias/add_item", payload)
	if err != nil {
		return "", false, fmt.Errorf("create URL table alias %s: %w", aliasName, err)
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, fmt.Errorf("URL table alias %s created but no UUID returned: %w", aliasName, err)
	}

	log.Printf("[Alias] created URL table alias %s (%s) -> %s", aliasName, uuid, serveURL)
	return uuid, true, nil
}
