//go:build !release

package main

import (
	"io/fs"
	"os"
)

func loadFrontendFS() (fsys fs.FS, mode string) {
	return os.DirFS("frontend/dist"), "disk"
}
