import * as wails from "/wails/runtime.js";

const $ = (id) => document.getElementById(id);
const call = (method, ...args) => wails.Call.ByName("main.App." + method, ...args);
const el = (tag, cls, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
};

let info = { text: {} };
const t = (key) => info.text[key] ?? key;
const maxLines = 400; // output lines kept per tool call

let live = false;   // a session is running
let idle = false;   // the agent waits for a message
const tools = new Map(); // tool call id -> {root, pre, mark, lines}

// ---------- home ----------

function showHome(error) {
  $("session").hidden = true;
  $("setup").hidden = true;
  $("home").hidden = false;
  $("home-error").hidden = !error;
  if (error) $("home-error").textContent = t("gui.start_failed") + "\n" + error;
  showProvider();
  $("ask-text").focus();
}

function showProvider() {
  const p = $("provider");
  p.textContent = info.ready ? info.provider : info.provider + " · " + t("gui.not_set");
  p.classList.toggle("warn", !info.ready);
  p.classList.toggle("muted", info.ready);
}

// ---------- setup ----------

let providers = [];

function selectedProvider() {
  const name = document.querySelector("#providers input:checked")?.value;
  return providers.find((p) => p.name === name);
}

// showFields shows what the chosen provider needs: a key, a URL or nothing.
function showFields() {
  const p = selectedProvider();
  $("key-row").hidden = !p || p.local;
  $("url-row").hidden = !p || !p.asksURL;
  $("setup-save").disabled = !p;
  if (!p) return;
  $("key").value = "";
  $("key").placeholder = p.hasKey ? t("gui.key_keep") : t("gui.key");
  $("key-link").textContent = t("gui.get_key") + " ↗";
  $("key-link").dataset.url = p.keyURL;
  $("url").value = p.baseURL ?? "";
}

async function showSetup() {
  $("home").hidden = true;
  $("session").hidden = true;
  $("setup").hidden = false;
  $("setup-result").hidden = true;
  await loadProviders();
}

async function loadProviders(keep) {
  providers = await call("Providers");
  const rows = providers.map((p) => {
    const row = el("label", "opt");
    const input = el("input");
    input.type = "radio";
    input.name = "provider";
    input.value = p.name;
    input.checked = keep ? p.name === keep : p.current;
    const body = el("div", "body");
    body.append(el("div", "strong", p.label));
    row.append(input, body);
    if (p.status) row.append(el("span", "badge" + (p.hasKey || p.local ? " good" : ""), p.status));
    if (p.current) row.append(el("span", "badge", t("gui.in_use")));
    return row;
  });
  $("providers").replaceChildren(...rows);
  showFields();
}

async function saveSetup() {
  const p = selectedProvider();
  if (!p) return;
  const out = $("setup-result");
  // The key leaves the field as soon as it is sent.
  const key = $("key").value;
  $("key").value = "";
  $("setup-save").disabled = true;
  out.hidden = false;
  out.className = "result";
  out.textContent = t("gui.saving");
  try {
    const r = await call("SaveSetup", p.name, p.local ? "" : key, p.asksURL ? $("url").value : "");
    const lines = [...(r.notes ?? []), t("gui.saved").replace("%s", p.label).replace(r.model ? "%s" : " · %s", r.model ?? "")];
    out.className = "result ok";
    out.textContent = lines.join("\n");
    info = await call("Info");
    await loadProviders(p.name);
  } catch (err) {
    out.className = "result bad";
    out.textContent = String(err?.message ?? err);
  }
  $("setup-save").disabled = false;
}

async function start(task, prompt) {
  $("home").hidden = true;
  $("session").hidden = false;
  $("log").replaceChildren();
  tools.clear();
  $("new").hidden = true;
  $("stop").hidden = false;
  $("mode").disabled = false;
  $("stats").textContent = "";
  $("s-task").textContent = "";
  $("s-model").textContent = "";
  live = true;
  setIdle(false);
  setState(t("gui.working"), true);
  if (prompt) add(el("div", "user", prompt));
  try {
    const s = await call("Start", task, prompt, $("dry").checked);
    $("s-task").textContent = s.task;
    $("s-model").textContent = s.provider + " · " + s.model;
    $("mode").value = s.mode;
  } catch (err) {
    live = false;
    showHome(String(err?.message ?? err));
  }
}

// ---------- session ----------

function add(node) {
  const log = $("log");
  const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 80;
  log.append(node);
  if (atEnd) log.scrollTop = log.scrollHeight;
  return node;
}

function setState(text, busy) {
  $("state").textContent = text;
  $("spinner").hidden = !busy;
}

function setIdle(on) {
  idle = on;
  $("reply-text").disabled = !on;
  $("reply-send").disabled = !on;
  if (on) $("reply-text").focus();
}

function seconds(s) {
  if (s < 60) return s.toFixed(s < 10 ? 1 : 0) + "s";
  return Math.floor(s / 60) + "m " + Math.round(s % 60) + "s";
}

function tokens(n) {
  return n >= 1e6 ? (n / 1e6).toFixed(1) + "M" : n >= 1e3 ? (n / 1e3).toFixed(1) + "k" : String(n);
}

function showStats(s) {
  const parts = [s.model, tokens(s.input + s.output) + " " + t("gui.tokens")];
  if (s.costKnown) parts.push("$" + s.cost.toFixed(s.cost < 1 ? 3 : 2));
  parts.push(seconds(s.seconds));
  $("stats").textContent = parts.join(" · ");
}

function toolRow(m) {
  const root = el("details", "tool");
  const sum = el("summary");
  sum.append(el("span", "name", m.tool), el("span", "arg", m.arg ?? ""));
  const mark = el("span", "mark");
  mark.append(el("span", "spinner"));
  sum.append(mark);
  const pre = el("pre");
  pre.hidden = true;
  root.append(sum, pre);
  if (m.command && m.command !== m.arg) sum.title = m.command;
  const entry = { root, pre, mark, lines: 0 };
  tools.set(m.id, entry);
  add(root);
  return entry;
}

function endTool(entry, m) {
  entry.root.classList.add(m.failed ? "bad" : "ok");
  let mark = m.failed ? "✗" : "✓";
  if (m.failed && m.exitCode) mark += " exit " + m.exitCode;
  if (m.seconds >= 1) mark += " " + seconds(m.seconds);
  entry.mark.textContent = mark;
  if (m.reason) {
    entry.root.append(el("div", "why", m.reason));
    entry.root.open = true;
  }
}

// card builds a request card. finish(text) closes it with the answer shown.
function card(title, badge) {
  const root = el("div", "card");
  const h = el("h3", "", title);
  if (badge) h.append(el("span", "badge " + badge, t("gui.risk." + (badge === "dangerous" ? "danger" : badge === "normal" ? "normal" : "safe"))));
  root.append(h);
  const finish = (text) => {
    root.classList.add("done");
    root.querySelectorAll("button, input").forEach((n) => (n.disabled = true));
    root.querySelector(".buttons")?.remove();
    root.querySelector(".row")?.remove();
    root.append(el("div", "answer", text));
    setState(t("gui.working"), true);
  };
  setState("", false);
  return { root, finish };
}

function button(label, cls, onClick) {
  const b = el("button", cls, label);
  b.type = "button";
  b.addEventListener("click", onClick);
  return b;
}

function confirmCard(m) {
  const c = card(t("ui.confirm"), m.risk);
  c.root.append(el("div", "cmd", m.command ?? ""));
  if (m.reason) c.root.append(el("div", "reason", m.reason));
  if (m.always) c.root.append(el("div", "warn", "⚠ " + t("ui.always_note")));
  const answer = (decision, label) => () => call("Confirm", m.id, decision).then(() => {
    c.finish(label);
    // "Stop asking" puts the session in automatic approval.
    if (decision === "all") $("mode").value = "auto";
  }, report);
  const buttons = el("div", "buttons");
  buttons.append(button(t("gui.approve"), "primary", answer("approve", t("gui.approve"))));
  if (!m.always) buttons.append(button(t("gui.approve_all"), "", answer("all", t("gui.approve_all"))));
  buttons.append(button(t("gui.deny"), "danger", answer("deny", t("gui.deny"))));
  c.root.append(buttons);
  add(c.root);
  buttons.firstChild.focus();
}

function choiceCard(m) {
  const c = card(m.text ?? "");
  const name = "choice-" + m.id;
  for (const ch of m.choices ?? []) {
    const row = el("label", "opt");
    const input = el("input");
    input.type = m.multi ? "checkbox" : "radio";
    input.name = name;
    input.value = ch.id;
    const body = el("div", "body");
    body.append(el("div", "strong", ch.label));
    if (ch.detail) body.append(el("div", "detail", ch.detail));
    row.append(input, body);
    if (ch.impact) row.append(el("span", "badge", ch.impact));
    if (ch.risk === "dangerous") row.append(el("span", "badge dangerous", t("gui.risk.danger")));
    c.root.append(row);
  }
  const picked = () => [...c.root.querySelectorAll("input:checked")].map((i) => i.value);
  const labels = (ids) => (m.choices ?? []).filter((ch) => ids.includes(ch.id)).map((ch) => ch.label).join(", ");
  const go = button(t("gui.choose"), "primary", () => {
    const ids = picked();
    call("Choose", m.id, ids).then(() => c.finish(labels(ids)), report);
  });
  go.disabled = true;
  c.root.addEventListener("change", () => (go.disabled = picked().length === 0));
  const buttons = el("div", "buttons");
  buttons.append(go, button(t("gui.choose_none"), "", () => call("Choose", m.id, []).then(() => c.finish(t("gui.choose_none")), report)));
  c.root.append(buttons);
  add(c.root);
}

function passwordCard(m) {
  const c = card("🔒 " + t("ui.password_title"));
  if (m.failed) c.root.append(el("div", "warn", t("ui.password_wrong")));
  if (m.text) c.root.append(el("div", "reason", m.text));
  const input = el("input");
  input.type = "password";
  input.placeholder = t("gui.password");
  input.autocomplete = "off";
  // The password goes to omega and nowhere else: the field is emptied as
  // soon as it is sent.
  const send = (secret) => {
    input.value = "";
    call("Password", m.id, secret).then(() => c.finish(secret ? "••••••" : t("gui.password_no")), report);
  };
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && input.value) send(input.value);
    if (e.key === "Escape") send("");
  });
  const row = el("div", "row");
  row.append(input, button(t("gui.password_ok"), "primary", () => input.value && send(input.value)));
  const buttons = el("div", "buttons");
  buttons.append(button(t("gui.password_no"), "", () => send("")));
  c.root.append(row, buttons);
  add(c.root);
  input.focus();
}

function report(err) {
  add(el("div", "note bad", "! " + String(err?.message ?? err)));
}

function handle(m) {
  if (m.stats) showStats(m.stats);
  switch (m.kind) {
    case "thinking":
      setState(t("gui.working"), true);
      break;
    case "assistant": {
      const d = el("div", "md");
      d.innerHTML = m.html ?? ""; // rendered by omega; raw HTML in the text is escaped there
      add(d);
      break;
    }
    case "tool_start":
      toolRow(m);
      setState(m.tool + (m.arg ? " " + m.arg : ""), true);
      break;
    case "output": {
      const e = tools.get(m.id);
      if (!e) break;
      e.pre.hidden = false;
      e.pre.append(m.text + "\n");
      if (++e.lines > maxLines) e.pre.firstChild?.remove();
      if (e.root.open) e.pre.scrollTop = e.pre.scrollHeight;
      break;
    }
    case "tool_end":
      endTool(tools.get(m.id) ?? toolRow(m), m);
      setState(t("gui.working"), true);
      break;
    case "denied": {
      const e = toolRow({ ...m, id: -Date.now() });
      e.root.classList.add("denied");
      e.mark.textContent = t("ui.denied");
      if (m.reason) e.root.append(el("div", "why", m.reason));
      break;
    }
    case "confirm": confirmCard(m); break;
    case "choice": choiceCard(m); break;
    case "password": passwordCard(m); break;
    case "model":
      $("s-model").textContent = $("s-model").textContent.split(" · ")[0] + " · " + m.text;
      add(el("div", "note", "↑ " + t("ui.model_switch").replace("%s", m.text)));
      break;
    case "mode":
      $("mode").value = m.mode;
      break;
    case "idle":
      setState("", false);
      setIdle(true);
      break;
    case "error":
      if (m.text?.trim()) add(el("div", "note bad", "! " + m.text.trim()));
      break;
    case "done":
      if (m.text) add(el("div", "note", m.text));
      break;
    case "closed":
      live = false;
      // Calls cut short by Stop never report an end.
      for (const e of tools.values()) e.mark.querySelector(".spinner")?.remove();
      setIdle(false);
      setState(t("gui.ended"), false);
      document.querySelectorAll(".card:not(.done)").forEach((c) => {
        c.classList.add("done");
        c.querySelectorAll("button, input").forEach((n) => (n.disabled = true));
      });
      $("stop").hidden = true;
      $("mode").disabled = true;
      $("new").hidden = false;
      break;
  }
}

// ---------- wiring ----------

async function init() {
  info = await call("Info");
  document.documentElement.lang = info.lang;
  $("tagline").textContent = info.tagline;
  $("version").textContent = "omega " + info.version;
  $("demo").hidden = !info.demo;
  $("demo").textContent = t("gui.demo");
  $("ask-text").placeholder = t("gui.ask");
  $("ask-send").textContent = t("gui.send");
  $("dry-label").textContent = t("gui.dry_run");
  $("reply-text").placeholder = t("gui.reply");
  $("reply-send").textContent = t("gui.send");
  $("stop").textContent = t("gui.stop");
  $("new").textContent = t("gui.new");
  $("setup-open").textContent = t("gui.setup_open");
  $("setup-title").textContent = t("gui.setup");
  $("setup-intro").textContent = t("gui.setup_intro");
  $("key-label").textContent = t("gui.key");
  $("url-label").textContent = t("gui.base_url");
  $("setup-save").textContent = t("gui.save");
  $("setup-back").textContent = t("gui.back");
  $("setup-open").addEventListener("click", () => showSetup().catch(report));
  $("setup-back").addEventListener("click", () => showHome());
  $("providers").addEventListener("change", () => {
    $("setup-result").hidden = true;
    showFields();
  });
  $("setup-form").addEventListener("submit", (e) => {
    e.preventDefault();
    saveSetup();
  });
  $("key-link").addEventListener("click", (e) => {
    e.preventDefault();
    const url = e.currentTarget.dataset.url;
    if (/^https?:\/\//.test(url ?? "")) wails.Browser.OpenURL(url);
  });
  for (const [value, key] of [["ask", "ui.mode_ask"], ["auto", "ui.mode_auto"], ["readonly", "ui.mode_readonly"]]) {
    const o = el("option", "", t(key));
    o.value = value;
    $("mode").append(o);
  }
  for (const task of info.tasks) {
    const b = el("button", "task");
    b.type = "button";
    b.append(el("b", "", task.title), el("span", "", task.detail));
    b.addEventListener("click", () => start(task.id, ""));
    $("tasks").append(b);
  }

  $("ask").addEventListener("submit", (e) => {
    e.preventDefault();
    const text = $("ask-text").value.trim();
    if (!text) return;
    $("ask-text").value = "";
    start("", text);
  });
  $("reply").addEventListener("submit", (e) => {
    e.preventDefault();
    const text = $("reply-text").value.trim();
    if (!text || !idle) return;
    $("reply-text").value = "";
    setIdle(false);
    add(el("div", "user", text));
    setState(t("gui.working"), true);
    call("Send", text).catch(report);
  });
  $("mode").addEventListener("change", () => {
    call("SetMode", $("mode").value).then((mode) => ($("mode").value = mode), report);
  });
  $("stop").addEventListener("click", () => call("Stop"));
  $("new").addEventListener("click", () => showHome());

  // Links in the model's text open in the browser, never in this window.
  $("log").addEventListener("click", (e) => {
    const a = e.target.closest?.("a[href]");
    if (!a) return;
    e.preventDefault();
    if (/^https?:\/\//.test(a.href)) wails.Browser.OpenURL(a.href);
  });

  wails.Events.On("omega", (ev) => handle(ev.data));
  showHome();
}

init().catch((err) => {
  document.body.textContent = "omega: " + String(err?.message ?? err);
});
