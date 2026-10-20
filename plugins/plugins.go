// Package plugins embeds the shipped plugin folders, so a binary without a
// plugins/ folder next to it (the Android app) still has every plugin.
// A plugins folder on disk (NATLAS_PLUGINS_DIR) always wins.
package plugins

import "embed"

// FS holds <id>/plugin.yml for every shipped plugin.
//
//go:embed */plugin.yml
var FS embed.FS
