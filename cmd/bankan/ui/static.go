package ui

import "embed"

// StaticFS contains all embedded static assets served at /static/*.
//
//go:embed static
var StaticFS embed.FS
