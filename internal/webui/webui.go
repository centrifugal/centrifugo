package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/dist
var static embed.FS
var FS http.FileSystem

func init() {
	dist, _ := fs.Sub(static, "web/dist")
	FS = http.FS(dist)
}
