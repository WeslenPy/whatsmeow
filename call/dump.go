// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	waBinary "go.mau.fi/whatsmeow/binary"
)

// DumpNode serializes a binary XML node to JSON for protocol analysis.
func DumpNode(node *waBinary.Node) ([]byte, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}
	return json.MarshalIndent(nodeToDump(node), "", "  ")
}

// DumpNodeString is a convenience wrapper around DumpNode.
func DumpNodeString(node *waBinary.Node) (string, error) {
	data, err := DumpNode(node)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type dumpNode struct {
	Tag     string         `json:"tag"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content any            `json:"content,omitempty"`
}

func nodeToDump(node *waBinary.Node) dumpNode {
	out := dumpNode{
		Tag:   node.Tag,
		Attrs: attrsToDump(node.Attrs),
	}
	switch content := node.Content.(type) {
	case nil:
	case []byte:
		out.Content = map[string]string{
			"type":   "bytes",
			"base64": base64.StdEncoding.EncodeToString(content),
			"length": fmt.Sprintf("%d", len(content)),
		}
	case []waBinary.Node:
		children := make([]dumpNode, len(content))
		for i, child := range content {
			children[i] = nodeToDump(&child)
		}
		out.Content = children
	default:
		out.Content = fmt.Sprintf("%v", content)
	}
	return out
}

func attrsToDump(attrs waBinary.Attrs) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for key, val := range attrs {
		out[key] = val
	}
	return out
}
