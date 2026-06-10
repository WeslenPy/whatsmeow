/**
 * Downloads baileys-caller WASM loader assets into binary/wasm and lib/.
 * Run once: npm run setup  (from cmd/voip-bridge)
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..", "..");
const wasmDir = path.join(repoRoot, "binary", "wasm");
const libDir = path.join(__dirname, "lib");

const FILES = [
  {
    url: "https://raw.githubusercontent.com/SheIITear/baileys-caller/main/dist/wasm-engine.mjs",
    dest: path.join(libDir, "wasm-engine.mjs"),
  },
  {
    url: "https://raw.githubusercontent.com/SheIITear/baileys-caller/main/dist/worker-bootstrap.mjs",
    dest: path.join(libDir, "worker-bootstrap.mjs"),
  },
  {
    url: "https://raw.githubusercontent.com/SheIITear/baileys-caller/main/assets/wasm/worker-modules.js",
    dest: path.join(wasmDir, "worker-modules.js"),
  },
  {
    url: "https://raw.githubusercontent.com/SheIITear/baileys-caller/main/assets/wasm/loader.js",
    dest: path.join(wasmDir, "loader.js"),
  },
];

const OPTIONAL_FILES = [
  {
    url: "https://media.githubusercontent.com/media/SheIITear/baileys-caller/main/assets/wasm/whatsapp.wasm",
    dest: path.join(wasmDir, "whatsapp.wasm"),
  },
];

async function download(url, dest) {
  console.log(`fetch ${url}`);
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status} for ${url}`);
  const buf = Buffer.from(await res.arrayBuffer());
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, buf);
  console.log(`  -> ${dest} (${buf.length} bytes)`);
}

async function main() {
  for (const f of FILES) {
    await download(f.url, f.dest);
  }
  for (const f of OPTIONAL_FILES) {
    if (fs.existsSync(f.dest)) {
      console.log(`skip ${f.dest} (already present)`);
      continue;
    }
    try {
      await download(f.url, f.dest);
    } catch (err) {
      console.warn(`  optional download failed: ${err.message}`);
    }
  }
  const wasmPath = path.join(wasmDir, "whatsapp.wasm");
  if (!fs.existsSync(wasmPath)) {
    console.warn(`missing ${wasmPath} — place a compatible whatsapp.wasm there manually`);
  }
  console.log("setup complete — WASM JS assets synced from baileys-caller");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
