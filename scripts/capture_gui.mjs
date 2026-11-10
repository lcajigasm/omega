#!/usr/bin/env node
// Record the desktop app's captures for the README and the website.
//
//   node scripts/capture_gui.mjs            # writes docs/assets/gui-*.png
//
// It builds cmd/omega-gui in Wails' server mode, which serves the same
// frontend and the same Go code over HTTP instead of opening a window, runs
// it with --demo (scripted session: no API calls, no real commands) and
// drives it in a headless Chromium through the DevTools protocol. Needs Go,
// Node 22 or later and Chrome, Chromium, Edge or Brave (or CHROME=/path).
// Configuration and state go to temporary folders, never to yours.
import { spawn, execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(root, "docs", "assets");
// A small window, so the text stays readable in the website's thumbnails;
// the same 1280:820 shape as the terminal captures.
const width = 890, height = 570, scale = 2;
const port = Number(process.env.OMEGA_CAPTURE_PORT ?? 34116);

function browser() {
  const found = [
    process.env.CHROME,
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
    "/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
  ].find((p) => p && existsSync(p));
  if (!found) throw new Error("capture: no Chromium browser found; set CHROME=/path/to/it");
  return found;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const tmp = mkdtempSync(join(tmpdir(), "omega-capture-"));
const children = [];
const cleanup = () => {
  for (const c of children) c.kill();
  // The browser may still be writing its profile while it exits.
  try {
    rmSync(tmp, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });
  } catch {}
};

async function main() {
  console.log("capture: building omega-gui (server mode)");
  const bin = join(tmp, "omega-gui-server");
  execFileSync("go", ["build", "-tags", "gui,server", "-ldflags", "-X main.version=capture", "-o", bin, "./cmd/omega-gui"],
    { cwd: root, stdio: "inherit", env: { ...process.env, CGO_ENABLED: "0" } });

  const app = spawn(bin, ["--demo"], {
    stdio: "ignore",
    env: {
      ...process.env, OMEGA_LANG: "en", WAILS_SERVER_PORT: String(port),
      XDG_CONFIG_HOME: join(tmp, "config"), XDG_STATE_HOME: join(tmp, "state"), XDG_CACHE_HOME: join(tmp, "cache"),
    },
  });
  children.push(app);

  const chrome = spawn(browser(), [
    "--headless=new", "--remote-debugging-port=0", `--user-data-dir=${join(tmp, "chrome")}`,
    "--no-first-run", "--no-default-browser-check", "--hide-scrollbars", "--disable-gpu", "about:blank",
  ], { stdio: ["ignore", "ignore", "pipe"] });
  children.push(chrome);
  const endpoint = await new Promise((resolve, reject) => {
    let err = "";
    chrome.stderr.on("data", (d) => {
      err += d;
      const m = err.match(/DevTools listening on (ws:\/\/\S+)/);
      if (m) resolve(m[1]);
    });
    chrome.on("exit", () => reject(new Error("capture: the browser exited\n" + err)));
    setTimeout(() => reject(new Error("capture: the browser did not start")), 20000);
  });

  // One flat DevTools session over a single socket.
  const ws = new WebSocket(endpoint);
  await new Promise((r, j) => { ws.onopen = r; ws.onerror = () => j(new Error("capture: no DevTools connection")); });
  let next = 0;
  const waiting = new Map();
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data);
    const w = waiting.get(m.id);
    if (!w) return;
    waiting.delete(m.id);
    m.error ? w.reject(new Error(m.error.message)) : w.resolve(m.result);
  };
  let sessionId;
  const send = (method, params = {}) => new Promise((resolve, reject) => {
    const id = ++next;
    waiting.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params, sessionId }));
  });

  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  ({ sessionId } = await send("Target.attachToTarget", { targetId, flatten: true }));
  await send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: scale, mobile: false });
  await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "dark" }] });

  const js = async (expression) => {
    const r = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error("capture: " + (r.exceptionDetails.exception?.description ?? r.exceptionDetails.text));
    return r.result.value;
  };
  const until = async (what, expression, ms = 60000) => {
    for (const end = Date.now() + ms; Date.now() < end; await sleep(150)) {
      if (await js(expression).catch(() => false)) return;
    }
    throw new Error("capture: timed out waiting for " + what);
  };
  const shot = async (name) => {
    await sleep(400); // let the last paint and the scroll settle
    const { data } = await send("Page.captureScreenshot", { format: "png" });
    writeFileSync(join(out, name), Buffer.from(data, "base64"));
    console.log("capture: docs/assets/" + name);
  };
  const click = (selector) => js(`document.querySelector(${JSON.stringify(selector)}).click()`);

  // The app answers once its server is up.
  for (const end = Date.now() + 20000; ; await sleep(200)) {
    if (await fetch(`http://localhost:${port}/`).then((r) => r.ok, () => false)) break;
    if (Date.now() > end) throw new Error("capture: omega-gui did not start on port " + port);
  }
  await send("Page.navigate", { url: `http://localhost:${port}/` });
  await until("the home screen", `document.querySelectorAll("#tasks button").length > 0`);
  await shot("gui-home.png");

  // Full maintenance: a step fails, omega diagnoses it and asks before the fix.
  await click("#tasks button");
  await until("the approval card", `!!document.querySelector(".card:not(.done) .cmd")`);
  await shot("gui-confirm.png");

  await click(".card:not(.done) button.primary");
  await until("the selection card", `document.querySelectorAll(".card:not(.done) input[type=checkbox]").length >= 4`);
  await js(`[...document.querySelectorAll(".card:not(.done) input[type=checkbox]")].forEach((c, i) => { if (i !== 2) c.click(); })`);
  await shot("gui-card.png");

  // Carry out the selection, approving whatever it asks, until omega reports.
  await click(".card:not(.done) button.primary");
  for (const end = Date.now() + 120000; ; await sleep(200)) {
    if (await js(`!document.getElementById("reply-text").disabled`)) break;
    if (Date.now() > end) throw new Error("capture: timed out waiting for the end of the session");
    await js(`document.querySelector(".card:not(.done) .buttons button.primary")?.click()`);
  }
  await shot("gui-summary.png");

  // Open the step that failed: its output is kept under the row.
  await js(`(() => { const d = document.querySelector(".tool.bad"); d.open = true; d.scrollIntoView({ block: "start" }); })()`);
  await shot("gui-output.png");

  await click("#stop");
  await until("the session to close", `!document.getElementById("new").hidden`);
  await click("#new");
  await click("#setup-open");
  await until("the setup screen", `document.querySelectorAll("#providers label").length > 0`);
  await shot("gui-setup.png");
  ws.close();
}

main().then(cleanup, (err) => {
  cleanup();
  console.error(String(err?.message ?? err));
  process.exit(1);
});
