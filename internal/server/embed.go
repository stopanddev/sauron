package server

import "embed"

// Static holds CSS and future assets for the hub (embedded for single-binary deploy).
//
//go:embed static
var Static embed.FS
