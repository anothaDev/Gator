//go:build release

package main

import (
	"embed"
	"io/fs"
	"log"
)

//go:embed frontend/dist
var embeddedFrontend embed.FS

func loadFrontendFS() (fsys fs.FS, mode string) {
	frontendFS, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		log.Fatal("Failed to load embedded frontend:", err)
	}
	return frontendFS, "embedded assets"
}
