// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import waBinary "go.mau.fi/whatsmeow/binary"

// ExtractReceiptCallID returns the call-id from a receipt node if present.
func ExtractReceiptCallID(node *waBinary.Node) string {
	if node == nil {
		return ""
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		if id := ag.OptionalString("call-id"); id != "" {
			return id
		}
	}
	ag := node.AttrGetter()
	return ag.OptionalString("call-id")
}
