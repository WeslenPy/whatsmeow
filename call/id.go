// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// GenerateCallID creates a WhatsApp call ID matching the baileys-caller format:
// "00" + upper-case hex of 15 random bytes (first random byte discarded).
func GenerateCallID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "00" + strings.ToUpper(hex.EncodeToString(buf)[2:]), nil
}
