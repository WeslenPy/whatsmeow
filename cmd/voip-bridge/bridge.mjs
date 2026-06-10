/**
 * WhatsApp VoIP WASM bridge for whatsmeow.
 * JSON-lines IPC on stdin/stdout between Go signaling and baileys-caller WasmEngine.
 */
import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = process.env.WASM_RESOURCES_PATH
  ? path.resolve(process.env.WASM_RESOURCES_PATH)
  : path.resolve(__dirname, "..", "..");

const wasmPath = process.env.WASM_PATH || path.join(repoRoot, "binary", "wasm", "whatsapp.wasm");
const enginePath = path.join(__dirname, "lib", "wasm-engine.mjs");
const debug = true;

/** Debug logs go to stderr — stdout is reserved for JSON IPC. */
function log(...args) {
  if (debug) {
    console.error("[voip-bridge]", ...args);
  }
}

function summarizeValue(value) {
  if (value == null) return value;
  if (typeof value === "string") {
    return value.length > 96 ? `<string ${value.length} chars>` : value;
  }
  if (value instanceof Uint8Array || Buffer.isBuffer(value)) {
    return `<bytes ${value.length}>`;
  }
  if (Array.isArray(value)) {
    return value.map(summarizeValue);
  }
  return value;
}

function summarizeReq(req) {
  const out = { id: req.id, op: req.op };
  for (const [key, value] of Object.entries(req)) {
    if (key === "id" || key === "op") continue;
    out[key] = summarizeValue(value);
  }
  return out;
}

function emit(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

function b64decode(s) {
  if (!s) return new Uint8Array(0);
  return Uint8Array.from(Buffer.from(s, "base64"));
}

function b64encode(buf) {
  if (!buf || buf.length === 0) return "";
  return Buffer.from(buf).toString("base64");
}

async function loadEngine() {
  const t0 = Date.now();
  log("loading WASM engine", { wasmPath, enginePath });
  if (!fs.existsSync(enginePath)) {
    throw new Error(
      `missing ${enginePath} — run: cd cmd/voip-bridge && npm run setup`,
    );
  }
  if (!fs.existsSync(wasmPath)) {
    throw new Error(`missing ${wasmPath}`);
  }
  const workerModules = path.join(repoRoot, "binary", "wasm", "worker-modules.js");
  if (!fs.existsSync(workerModules)) {
    throw new Error(
      `missing ${workerModules} — run: cd cmd/voip-bridge && npm run setup`,
    );
  }

  const wasmDir = path.join(repoRoot, "binary", "wasm");
  const workerModulesCode = fs.readFileSync(path.join(wasmDir, "worker-modules.js"), "utf8");
  // worker-modules.js embeds WAWebVoipWebWasmLoader — do NOT also load standalone loader.js
  // (older build clobbers env imports like on_call_event_js_sync).
  const workerBundleHasLoader = /WAWebVoipWebWasmLoader/.test(workerModulesCode);
  log("worker bundle", {
    workerModulesBytes: workerModulesCode.length,
    workerBundleHasLoader,
    loaderSkipped: workerBundleHasLoader,
  });
  let loaderCode = "";
  if (!workerBundleHasLoader) {
    const loaderFile = path.join(wasmDir, "loader.js");
    if (fs.existsSync(loaderFile)) {
      loaderCode = fs.readFileSync(loaderFile, "utf8");
    }
  }

  const { WasmEngine } = await import(pathToFileURL(enginePath).href);
  const engine = new WasmEngine({
    resourcesPath: path.join(__dirname, "lib"),
    wasmPath,
    workerModulesCode,
    loaderCode,
    loaderModuleName: "WAWebVoipWebWasmLoader.worker",
    enableLogs: true,
    callbacks: {
      onSignalingXmpp: (peerJid, callId, xmlPayload) => {
        log("WASM onSignalingXmpp", {
          peerJid,
          callId,
          payloadBytes: xmlPayload?.length ?? 0,
        });
        emit({
          event: "onSignalingXmpp",
          peerJid,
          callId,
          payload: b64encode(xmlPayload),
        });
      },
      onCallEvent: (eventType, eventData) => {
        log("WASM onCallEvent", {
          eventType,
          eventDataBytes: (eventData ?? "").length,
        });
        emit({ event: "onCallEvent", eventType, eventData: eventData ?? "" });
      },
      sendDataToRelay: (data, ip, port) => {
        log("WASM sendDataToRelay", { ip, port, bytes: data?.length ?? 0 });
        emit({
          event: "sendDataToRelay",
          ip,
          port,
          data: b64encode(data),
        });
        return 0;
      },
      onLog: (level, message) => {
        if (debug) {
          console.error(`[voip-bridge][wasm:${level}]`, message);
          emit({ event: "log", level, message });
        }
      },
    },
  });

  await engine.initialize();
  log("WASM engine initialized", { ms: Date.now() - t0 });
  return engine;
}

/** @type {import('./lib/wasm-engine.mjs').WasmEngine | null} */
let engine = null;

const pending = new Map();

async function handle(req) {
  const { id, op } = req;
  const t0 = Date.now();
  log("rpc ->", summarizeReq(req));
  try {
    switch (op) {
      case "init": {
        if (!engine) engine = await loadEngine();
        const ownPN = req.ownPN || "";
        const ownLID = req.ownLID || ownPN;
        const barePN = ownPN.split(":")[0].split("@")[0] + "@s.whatsapp.net";
        log("initVoipStack", { ownPN, barePN, ownLID });
        engine.initVoipStack(ownPN, barePN, ownLID);
        await engine.waitForVoipStackReady();
        log("voip stack ready", { ms: Date.now() - t0 });
        emit({ id, ok: true });
        break;
      }
      case "startCall": {
        if (!engine) throw new Error("not initialized");
        engine.startCall({
          peerJid: req.peerJid,
          peerPn: req.peerPn,
          peerList: req.peerList,
          callId: req.callId,
          isVideo: !!req.isVideo,
          isLidCall: req.isLidCall !== false,
          extraData: b64decode(req.tcToken),
        });
        emit({ id, ok: true });
        break;
      }
      case "endCall": {
        if (engine) engine.endCall(req.reason ?? 0, req.sendTerminate !== false);
        emit({ id, ok: true });
        break;
      }
      case "handleSignalingAck": {
        if (!engine) throw new Error("not initialized");
        engine.handleSignalingAck({
          payload: req.payload,
          ackError: req.ackError ?? "0",
          msgType: req.msgType ?? "",
          peerJid: req.peerJid ?? "",
          extraData: b64decode(req.tcToken),
        });
        emit({ id, ok: true });
        break;
      }
      case "handleSignalingMessage": {
        if (!engine) throw new Error("not initialized");
        engine.handleSignalingMessage({
          payload: req.payload,
          peerPlatform: req.peerPlatform,
          peerAppVersion: req.peerAppVersion,
          epochId: req.epochId,
          timestamp: req.timestamp,
          isOffline: !!req.isOffline,
          peerJid: req.peerJid,
          tcToken: b64decode(req.tcToken),
        });
        emit({ id, ok: true });
        break;
      }
      case "handleSignalingOffer": {
        if (!engine) throw new Error("not initialized");
        engine.handleSignalingOffer({
          payload: req.payload,
          peerPlatform: Number(req.peerPlatform || 0),
          peerAppVersion: req.peerAppVersion,
          epochId: req.epochId,
          timestamp: req.timestamp,
          isOffline: !!req.isOffline,
          isOfferNotContact: !!req.isOfferNotContact,
          peerJid: req.peerJid,
          tcToken: b64decode(req.tcToken),
        });
        emit({ id, ok: true });
        break;
      }
      case "handleSignalingReceipt": {
        if (!engine) throw new Error("not initialized");
        engine.handleSignalingReceipt({
          payload: req.payload,
          peerJid: req.peerJid,
          tcToken: b64decode(req.tcToken),
        });
        emit({ id, ok: true });
        break;
      }
      case "handleTransportMessage": {
        if (!engine) throw new Error("not initialized");
        engine.handleOnTransportMessage(b64decode(req.data), req.ip, req.port);
        emit({ id, ok: true });
        break;
      }
      case "shutdown": {
        if (engine) {
          engine.destroy();
          engine = null;
        }
        emit({ id, ok: true });
        process.exit(0);
        break;
      }
      default:
        throw new Error(`unknown op: ${op}`);
    }
    log("rpc <-", { id, op, ok: true, ms: Date.now() - t0 });
  } catch (err) {
    const message = String(err?.message || err);
    console.error("[voip-bridge] rpc error", { id, op, error: message });
    log("rpc <-", { id, op, ok: false, ms: Date.now() - t0, error: message });
    emit({ id, ok: false, error: message });
  }
}

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  const trimmed = line.trim();
  if (!trimmed) return;
  let req;
  try {
    req = JSON.parse(trimmed);
  } catch (err) {
    console.error("[voip-bridge] invalid json:", err);
    emit({ id: 0, ok: false, error: `invalid json: ${err}` });
    return;
  }
  void handle(req);
});

process.on("SIGTERM", () => {
  log("SIGTERM — shutting down");
  if (engine) engine.destroy();
  process.exit(0);
});

emit({ event: "ready", repoRoot, wasmPath, debug });
if (debug) {
  console.error("[voip-bridge] ready (debug enabled)", { repoRoot, wasmPath });
}
