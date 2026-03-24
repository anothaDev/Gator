package main

import (
	"io/fs"
	"net/http"
	pathpkg "path"
	"strings"

	"github.com/gin-gonic/gin"
)

func serveFrontend(r *gin.Engine) {
	frontendFS, mode := loadFrontendFS()
	index, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		panic("frontend index missing: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(frontendFS))

	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		name := cleanedFrontendPath(c.Request.URL.Path)
		if name == "index.html" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", index)
			return
		}

		if fileExists(frontendFS, name) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		if strings.Contains(pathpkg.Base(name), ".") {
			c.Status(http.StatusNotFound)
			return
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	gin.DefaultWriter.Write([]byte("Serving frontend from " + mode + "\n"))
}

func cleanedFrontendPath(urlPath string) string {
	cleaned := pathpkg.Clean("/" + urlPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "index.html"
	}
	return cleaned
}

func fileExists(frontendFS fs.FS, name string) bool {
	info, err := fs.Stat(frontendFS, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
