// Package web exposes the compiled client. Run npm ci && npm run build in web first.
package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var compiled embed.FS

func Assets() fs.FS {
	assets, err := fs.Sub(compiled, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
