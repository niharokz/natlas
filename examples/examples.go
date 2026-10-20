// Package examples embeds the sample data, so the phone app can offer a demo
// folder without any download.
package examples

import "embed"

// Data holds data/<file> (event.md, subscription.md, …).
//
//go:embed data/*
var Data embed.FS
