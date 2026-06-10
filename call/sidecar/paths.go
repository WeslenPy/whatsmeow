// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sidecar

import (
	"os"
	"path/filepath"
)

// DefaultBridgeScript returns the path to cmd/voip-bridge/bridge.mjs inside the module.
func DefaultBridgeScript() string {
	if p := os.Getenv("CALL_VOIP_BRIDGE_PATH"); p != "" {
		return p
	}
	if root := findModuleRoot(); root != "" {
		return filepath.Join(root, "cmd", "voip-bridge", "bridge.mjs")
	}
	return filepath.Join("cmd", "voip-bridge", "bridge.mjs")
}

// DefaultNodeCommand returns the Node.js executable (NODE_PATH env override).
func DefaultNodeCommand() string {
	if p := os.Getenv("NODE_PATH"); p != "" {
		return p
	}
	return "node"
}

func findModuleRoot() string {
	if p := os.Getenv("WHATSMEOW_ROOT"); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
