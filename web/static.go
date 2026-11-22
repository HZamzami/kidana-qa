// Package web embeds the static assets (fonts, images, vendored JS, built CSS)
// so the production binary is self-contained.
package web

import "embed"

//go:embed static
var Static embed.FS
