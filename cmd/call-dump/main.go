// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command call-dump connects to WhatsApp and dumps inbound call signaling nodes to JSON files.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	outDir := "call-dumps"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	dbLog := waLog.Stdout("Database", "INFO", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:test_account.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(device, clientLog)
	client.AddEventHandler(func(evt any) {
		dumpCallEvent(outDir, evt)
	})

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx)
		if err = client.Connect(); err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("Scan QR:", evt.Code)
			} else {
				fmt.Println("Login:", evt.Event)
			}
		}
	} else if err = client.Connect(); err != nil {
		panic(err)
	}

	fmt.Printf("Listening for call events. Dumps go to %s\n", outDir)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	client.Disconnect()
}

func dumpCallEvent(outDir string, evt any) {
	var node *waBinary.Node
	var name string
	switch v := evt.(type) {
	case *events.CallOffer:
		name = "offer"
		node = v.Data
	case *events.CallPreAccept:
		name = "preaccept"
		node = v.Data
	case *events.CallAccept:
		name = "accept"
		node = v.Data
	case *events.CallTransport:
		name = "transport"
		node = v.Data
	case *events.CallRelayLatency:
		name = "relaylatency"
		node = v.Data
	case *events.CallTerminate:
		name = "terminate"
		node = v.Data
	case *events.CallReject:
		name = "reject"
		node = v.Data
	default:
		return
	}
	raw, err := whatsmeow.DumpCallNode(node)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dump error: %v\n", err)
		return
	}
	filename := filepath.Join(outDir, fmt.Sprintf("%s-%d.json", name, time.Now().UnixNano()))
	if err = os.WriteFile(filename, raw, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		return
	}
	fmt.Println("dumped", filename)
}
