// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command call-test places an outbound call (signaling only) to a phone number.
//
// Usage:
//
//	go run ./cmd/call-test 5511999999999
//	go run ./cmd/call-test 5511999999999 30   # auto-cancel after 30s
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"

	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/call"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <phone-number>\n", os.Args[0])
		os.Exit(1)
	}
	phone := os.Args[1]
	var autoCancel time.Duration
	if len(os.Args) > 2 {
		secs, err := strconv.Atoi(os.Args[2])
		if err != nil || secs <= 0 {
			fmt.Fprintf(os.Stderr, "invalid timeout seconds: %s\n", os.Args[2])
			os.Exit(1)
		}
		autoCancel = time.Duration(secs) * time.Second
	}

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:test_account.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(device, clientLog)
	voipMode := os.Getenv("CALL_SIGNALING_ONLY") != "1"
	if !voipMode {
		client.EnableCallSupport(call.DefaultOptions())
	}

	client.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.CallOutgoing:
			fmt.Println("call outgoing:", v.CallID)
		case *events.CallRinging:
			fmt.Println("call ringing:", v.CallID)
		case *events.CallSignalingConnected:
			fmt.Println("call signaling connected:", v.CallID)
		case *events.CallEnded:
			fmt.Println("call ended:", v.CallID, v.Reason)
		}
	})

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx)
		if err = client.Connect(); err != nil {
			panic(err)
		}
		for evt := range qrChan {
			switch evt.Event {
			case whatsmeow.QRChannelEventCode:
				fmt.Println("Escaneie o QR code abaixo com o WhatsApp:")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			case "success":
				fmt.Println("Login realizado com sucesso!")
			default:
				fmt.Println("Evento de login:", evt.Event)
			}
		}
	} else if err = client.Connect(); err != nil {
		panic(err)
	}

	if client.Store.ID == nil {
		fmt.Fprintln(os.Stderr, "Nao foi possivel autenticar. Rode primeiro: go run ./test_account")
		os.Exit(1)
	}
	if !client.WaitForConnection(30 * time.Second) {
		fmt.Fprintln(os.Stderr, "Timeout aguardando conexao com o WhatsApp.")
		os.Exit(1)
	}

	if err = client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		fmt.Println("warning: send presence:", err)
	}

	if voipMode {
		fmt.Println("VoIP mode: Node sidecar + relay (cmd/voip-bridge)")
		fmt.Println("Loading WASM engine (may take ~30s)...")
		if err = client.EnableVoIP(ctx); err != nil {
			panic(err)
		}
		fmt.Println("VoIP engine ready")
	}

	peer := types.JID{User: phone, Server: types.DefaultUserServer}
	outbound, err := client.InitiateCall(ctx, peer, whatsmeow.OutgoingCallOptions{})
	if err != nil {
		panic(err)
	}
	if autoCancel > 0 {
		fmt.Printf("Calling %s (call-id %s). Auto-cancel in %s. Ctrl+C to cancel now.\n", phone, outbound.CallID(), autoCancel)
	} else {
		fmt.Printf("Calling %s (call-id %s). Press Ctrl+C to cancel.\n", phone, outbound.CallID())
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		if autoCancel > 0 {
			timer := time.NewTimer(autoCancel)
			defer timer.Stop()
			select {
			case <-c:
				_ = outbound.Cancel(context.Background())
			case <-timer.C:
				fmt.Println("auto-cancel timeout reached")
				_ = outbound.Cancel(context.Background())
			}
			return
		}
		<-c
		_ = outbound.Cancel(context.Background())
	}()

	reason, err := outbound.Wait(ctx)
	if err != nil {
		fmt.Println("wait error:", err)
	}
	fmt.Println("finished:", reason)
	client.Disconnect()
}
