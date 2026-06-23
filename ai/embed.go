// Package ai embeds installable AI agent bundles shipped with the remotr CLI.
package ai

import "embed"

// RemotrAgent is the remotr operator skill bundle for Claude, Cursor, and compatible agents.
//
//go:embed remotr-agent/*
var RemotrAgent embed.FS

// BundleRoot is the directory name inside the embedded filesystem.
const BundleRoot = "remotr-agent"
