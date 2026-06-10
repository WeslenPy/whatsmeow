# VoIP Bridge (Node.js sidecar)

Bridges whatsmeow Go signaling with the WhatsApp Web VoIP WASM stack
(baileys-caller `WasmEngine`).

## Setup (once)

```bash
cd cmd/voip-bridge
npm run setup
```

Downloads `wasm-engine.mjs`, `worker-bootstrap.mjs`, `worker-modules.js`, and
`loader.js` into this repo. Requires `binary/wasm/whatsapp.wasm` to already exist.

## Requirements

- Node.js >= 20
- `binary/wasm/whatsapp.wasm` (~9.8 MB)

## Manual test

```bash
node bridge.mjs
# stdin: {"id":1,"op":"init","ownPN":"...","ownLID":"..."}
```

## Debug logging

Set `VOIP_BRIDGE_DEBUG=1`. Logs go to **stderr** (stdout stays JSON IPC).

```bash
# bridge only
npm run start:debug

# full call-test from repo root (PowerShell)
$env:VOIP_BRIDGE_DEBUG="1"; go run ./cmd/call-test 5511999999999
```

Debug output includes RPC ops, WASM callbacks (`onSignalingXmpp`, `onCallEvent`,
`sendDataToRelay`), load timings, and internal WASM log lines.

## Go integration

```go
client.EnableVoIP(ctx)
```

Spawns `node cmd/voip-bridge/bridge.mjs` automatically. Go does not load WASM directly.

Environment variables:

| Variable | Purpose |
|----------|---------|
| `CALL_VOIP_BRIDGE_PATH` | Override bridge script path |
| `NODE_PATH` | Override node executable |
| `WASM_RESOURCES_PATH` | Repo root (auto-detected) |
| `VOIP_BRIDGE_DEBUG` | `1` for verbose bridge logs |
