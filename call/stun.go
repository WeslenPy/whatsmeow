// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package call

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
)

const (
	stunMagicCookie      = 0x2112A442
	stunBindingRequest   = 0x0001
	stunAttrWAToken      = 0x4000
	stunAttrWASession    = 0x4002
	stunAttrXORMappedAddr = 0x0020
	stunLatencyBase      = 33554432
)

// STUNResult holds the result of a relay STUN binding.
type STUNResult struct {
	RelayName    string
	RelayAddr    string
	RTT          time.Duration
	ResponseType uint16
	ResponseSize int
	MappedIP     net.IP
	MappedPort   uint16
	SessionData  []byte
}

// PingRelay sends a STUN Binding request with a WhatsApp token to a relay server.
func PingRelay(ep ParsedRelayEndpoint, token, hmacKey []byte, timeout time.Duration) (*STUNResult, error) {
	if token == nil {
		return nil, fmt.Errorf("nil token for relay %s", ep.RelayName)
	}
	if ep.IP == nil {
		return nil, fmt.Errorf("relay %s has no IP", ep.RelayName)
	}

	addr := net.JoinHostPort(ep.IP.String(), fmt.Sprintf("%d", ep.Port))
	conn, err := net.DialTimeout("udp4", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial relay %s: %w", ep.RelayName, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	txID := make([]byte, 12)
	_, _ = rand.Read(txID)

	msg := buildSTUNMessage(stunBindingRequest, txID, []stunAttr{{Type: stunAttrWAToken, Value: token}})
	if len(hmacKey) > 0 {
		msg = appendMessageIntegrity(msg, hmacKey)
	}

	start := time.Now()
	if _, err = conn.Write(msg); err != nil {
		return nil, fmt.Errorf("write relay %s: %w", ep.RelayName, err)
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read relay %s: %w", ep.RelayName, err)
	}
	rtt := time.Since(start)

	result := &STUNResult{
		RelayName:    ep.RelayName,
		RelayAddr:    addr,
		RTT:          rtt,
		ResponseType: binary.BigEndian.Uint16(buf[0:2]),
		ResponseSize: n,
	}
	attrs, err := parseSTUNResponse(buf[:n], txID)
	if err != nil {
		return result, err
	}
	for _, attr := range attrs {
		switch attr.Type {
		case stunAttrXORMappedAddr:
			result.MappedIP, result.MappedPort = decodeXORMappedAddr(attr.Value, txID)
		case stunAttrWASession:
			result.SessionData = append([]byte(nil), attr.Value...)
		}
	}
	return result, nil
}

// PingRelays pings unique relay endpoints in parallel.
func PingRelays(relay *ParsedRelay, timeout time.Duration) []*STUNResult {
	if relay == nil {
		return nil
	}
	hmacKey := relay.HBHKeyRaw
	if len(hmacKey) == 0 {
		hmacKey = relay.KeyRaw
	}

	seen := make(map[string]bool)
	ch := make(chan *STUNResult, len(relay.Endpoints))
	launched := 0
	for _, ep := range relay.Endpoints {
		if seen[ep.RelayName] || ep.IP == nil {
			continue
		}
		seen[ep.RelayName] = true
		token := relay.Tokens[ep.TokenID]
		if token == nil {
			continue
		}
		launched++
		go func(ep ParsedRelayEndpoint, tok []byte) {
			res, err := PingRelay(ep, tok, hmacKey, timeout)
			if err == nil {
				ch <- res
			} else {
				ch <- nil
			}
		}(ep, token)
	}

	results := make([]*STUNResult, 0, launched)
	for i := 0; i < launched; i++ {
		if res := <-ch; res != nil {
			results = append(results, res)
		}
	}
	return results
}

// RelayLatencyTE builds te nodes for a relaylatency stanza from STUN results.
func RelayLatencyTE(results []*STUNResult) []waBinary.Node {
	var nodes []waBinary.Node
	for _, r := range results {
		if r == nil || r.MappedIP == nil {
			continue
		}
		host, portStr, _ := net.SplitHostPort(r.RelayAddr)
		relayIP := net.ParseIP(host)
		if relayIP == nil {
			continue
		}
		var port uint16
		fmt.Sscanf(portStr, "%d", &port)
		ipPort := encodeIPPort(relayIP, port)
		if ipPort == nil {
			continue
		}
		rttMs := int(r.RTT.Milliseconds())
		nodes = append(nodes, waBinary.Node{
			Tag: "te",
			Attrs: waBinary.Attrs{
				"latency":    fmt.Sprintf("%d", stunLatencyBase+rttMs),
				"relay_name": r.RelayName,
			},
			Content: ipPort,
		})
	}
	return nodes
}

func encodeIPPort(ip net.IP, port uint16) []byte {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return nil
	}
	buf := make([]byte, 6)
	copy(buf[:4], ipv4)
	binary.BigEndian.PutUint16(buf[4:], port)
	return buf
}

type stunAttr struct {
	Type  uint16
	Value []byte
}

func buildSTUNMessage(msgType uint16, txID []byte, attrs []stunAttr) []byte {
	bodyLen := 0
	for _, attr := range attrs {
		bodyLen += 4 + len(attr.Value)
		if pad := len(attr.Value) % 4; pad != 0 {
			bodyLen += 4 - pad
		}
	}
	msg := make([]byte, 20+bodyLen)
	binary.BigEndian.PutUint16(msg[0:2], msgType)
	binary.BigEndian.PutUint16(msg[2:4], uint16(bodyLen))
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID)
	offset := 20
	for _, attr := range attrs {
		binary.BigEndian.PutUint16(msg[offset:offset+2], attr.Type)
		binary.BigEndian.PutUint16(msg[offset+2:offset+4], uint16(len(attr.Value)))
		copy(msg[offset+4:], attr.Value)
		offset += 4 + len(attr.Value)
		if pad := len(attr.Value) % 4; pad != 0 {
			offset += 4 - pad
		}
	}
	return msg
}

func parseSTUNResponse(data, expectedTxID []byte) ([]stunAttr, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("response too short")
	}
	if binary.BigEndian.Uint32(data[4:8]) != stunMagicCookie {
		return nil, fmt.Errorf("invalid magic cookie")
	}
	for i := range 12 {
		if data[8+i] != expectedTxID[i] {
			return nil, fmt.Errorf("transaction ID mismatch")
		}
	}
	bodyLen := int(binary.BigEndian.Uint16(data[2:4]))
	var attrs []stunAttr
	offset := 20
	for offset+4 <= 20+bodyLen && offset+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if offset+4+attrLen > len(data) {
			break
		}
		attrs = append(attrs, stunAttr{Type: attrType, Value: data[offset+4 : offset+4+attrLen]})
		offset += 4 + attrLen
		if pad := attrLen % 4; pad != 0 {
			offset += 4 - pad
		}
	}
	return attrs, nil
}

func decodeXORMappedAddr(data, txID []byte) (net.IP, uint16) {
	if len(data) < 8 || data[1] != 0x01 {
		return nil, 0
	}
	xPort := binary.BigEndian.Uint16(data[2:4])
	port := xPort ^ uint16(stunMagicCookie>>16)
	ip := make(net.IP, 4)
	cookieBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cookieBytes, stunMagicCookie)
	for i := range 4 {
		ip[i] = data[4+i] ^ cookieBytes[i]
	}
	_ = txID
	return ip, port
}

func appendMessageIntegrity(msg, key []byte) []byte {
	const miAttrType = 0x0008
	const miAttrLen = 20
	const miTotalLen = 4 + miAttrLen
	currentBodyLen := binary.BigEndian.Uint16(msg[2:4])
	binary.BigEndian.PutUint16(msg[2:4], currentBodyLen+miTotalLen)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	digest := mac.Sum(nil)
	attr := make([]byte, miTotalLen)
	binary.BigEndian.PutUint16(attr[0:2], miAttrType)
	binary.BigEndian.PutUint16(attr[2:4], miAttrLen)
	copy(attr[4:], digest)
	return append(msg, attr...)
}
