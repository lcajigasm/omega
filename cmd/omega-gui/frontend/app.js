import * as wails from "/wails/runtime.js";

const $ = (id) => document.getElementById(id);
const call = (method, ...args) => wails.Call.ByName("main.App." + method, ...args);
const el = (tag, cls, text) => {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
};
const message = (err) => String(err?.message ?? err);

let info = { text: {}, tasks: [], languages: [] };
const t = (key) => info.text[key] ?? key;
const maxLines = 400; // output lines kept per tool call

// ---------- icons ----------

const icons = {
  home: '<path d="M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z"/><path d="M19 15v4M17 17h4"/>',
  session: '<path d="M4 5h16v11H9l-5 4z"/>',
  history: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  system: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
  settings: '<path d="M4 7h10M18 7h2M4 17h2M10 17h10"/><circle cx="16" cy="7" r="2"/><circle cx="8" cy="17" r="2"/>',
  update: '<circle cx="12" cy="12" r="9"/><path d="M12 16V8M8.5 11.5L12 8l3.5 3.5"/>',
  clean: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v5M14 11v5"/>',
  analyze: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>',
  health: '<path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"/>',
  fix: '<path d="M14.7 6.3a4 4 0 0 0-5.4 5.4L3 18l3 3 6.3-6.3a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.4-.6-.6-2.4z"/>',
  tune: '<path d="M6 4v6M6 14v6M12 4v2M12 10v10M18 4v10M18 18v2"/><circle cx="6" cy="12" r="2"/><circle cx="12" cy="8" r="2"/><circle cx="18" cy="16" r="2"/>',
  dev: '<path d="M8 7l-5 5 5 5M16 7l5 5-5 5M14 4l-4 16"/>',
  mac: '<path d="M5 5h14v10H5zM2 19h20"/>',
  docker: '<path d="M3 8l9-5 9 5v8l-9 5-9-5zM3 8l9 5 9-5M12 13v8"/>',
};
icons.full = icons.home;

function icon(name) {
  const span = el("span");
  span.innerHTML = `<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">${icons[name] ?? icons.home}</svg>`;
  return span.firstChild;
}

// ---------- texts and navigation ----------

// applyTexts fills every element that names a message. It runs again when
// the language changes.
function applyTexts() {
  document.documentElement.lang = info.lang;
  for (const n of document.querySelectorAll("[data-t]")) n.textContent = t(n.dataset.t);
  for (const n of document.querySelectorAll("[data-ph]")) n.placeholder = t(n.dataset.ph);
  for (const n of document.querySelectorAll("[data-title]")) n.title = t(n.dataset.title);
  for (const b of document.querySelectorAll("#nav button[data-view]")) b.title = b.querySelector(".label").textContent;
  $("tagline").textContent = info.tagline;
  $("version").textContent = "omega " + info.version;
  $("demo").hidden = !info.demo;
  renderTasks();
  showProvider();
}

const views = ["home", "session", "history", "system", "settings"];
const loaders = { home: showHome, session: showSession, history: showHistory, system: showSystem, settings: showSettings };
let view = "";

function go(name) {
  view = name;
  for (const v of views) $(v).hidden = v !== name;
  for (const b of document.querySelectorAll("#nav button[data-view]")) b.classList.toggle("on", b.dataset.view === name);
  Promise.resolve(loaders[name]()).catch(toastError);
}

let toastTimer;
function toast(text, bad) {
  const n = $("toast");
  n.textContent = text;
  n.className = "toast" + (bad ? " bad" : "");
  n.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (n.hidden = true), bad ? 6000 : 1600);
}
const toastError = (err) => toast(message(err), true);

function showProvider() {
  const text = info.ready || info.demo ? info.provider : info.provider + " · " + t("gui.not_set");
  $("provider").textContent = text;
  $("nav-provider").title = text;
  $("provider-dot").classList.toggle("warn", !info.ready && !info.demo);
  $("fact-provider").textContent = text;
  $("fact-last").textContent = info.lastRun || t("gui.home.never");
}

async function refreshInfo() {
  info = await call("Info");
  showProvider();
}

// ---------- home ----------

let live = false;    // a session is running
let started = false; // a session has been shown since the app opened
let idle = false;    // the agent waits for a message
let asking = 0;      // cards waiting for an answer
let startError = "";
const tools = new Map(); // tool call id -> {root, pre, mark, lines}

const canStart = () => !live && (info.ready || info.demo);

function renderTasks() {
  const [lead, ...rest] = info.tasks;
  if (!lead) return;
  $("lead").dataset.task = lead.id;
  $("lead-title").textContent = lead.title;
  $("lead-detail").textContent = lead.detail;
  $("tasks").replaceChildren(...rest.map((task) => {
    const b = el("button", "task");
    b.type = "button";
    b.dataset.task = task.id;
    const body = el("span", "body");
    body.append(el("b", "", task.title), el("span", "", task.detail));
    b.append(icon(task.id), body);
    return b;
  }));
  homeState();
}

// homeState says why nothing can start, when it cannot, and where to go.
function homeState() {
  const alert = $("home-alert");
  let text = "", action = "", target = "";
  if (live) [text, action, target] = [t("gui.home.running"), t("gui.home.view"), "session"];
  else if (startError) [text, action, target] = [t("gui.start_failed") + "\n" + startError, t("gui.setup_open"), "settings"];
  else if (!info.ready && !info.demo) [text, action, target] = [t("gui.home.need_setup"), t("gui.setup_open"), "settings"];
  alert.hidden = !text;
  alert.classList.toggle("bad", !live && !!startError);
  $("home-alert-text").textContent = text;
  $("home-alert-go").textContent = action;
  $("home-alert-go").dataset.go = target;
  for (const b of document.querySelectorAll("#home [data-task], #ask input, #ask button, #dry")) b.disabled = !canStart();
}

async function showHome() {
  homeState();
  if (canStart()) $("ask-text").focus({ preventScroll: true });
  await refreshInfo();
  homeState();
}

async function start(task, prompt) {
  if (!canStart()) return;
  const preset = info.tasks.find((x) => x.id === task);
  $("log").replaceChildren();
  tools.clear();
  $("new").hidden = true;
  $("stop").hidden = $("mode").hidden = $("reply").hidden = false;
  $("stats").textContent = "";
  $("s-task").textContent = prompt || preset?.title || task;
  $("s-model").textContent = "";
  live = started = true;
  asking = 0;
  startError = "";
  setMode("");
  setIdle(false);
  setState(t("gui.working"), true);
  if (prompt) add(el("div", "user", prompt));
  go("session");
  try {
    const s = await call("Start", task, prompt, $("dry").checked);
    $("s-model").textContent = s.provider + " · " + s.model;
    setMode(s.mode);
  } catch (err) {
    live = started = false;
    startError = message(err);
    go("home");
  }
  badge();
}

// ---------- session ----------

function showSession() {
  $("session-none").hidden = started;
  $("session-body").hidden = !started;
  if (started) {
    const log = $("log");
    log.scrollTop = log.scrollHeight;
    if (idle) $("reply-text").focus();
  }
}

// badge marks the session in the sidebar: working, or waiting for an answer.
function badge() {
  const b = $("nav-badge");
  b.hidden = !live;
  b.className = "badge-dot " + (asking > 0 || idle ? "ask" : "busy");
}

function add(node) {
  const log = $("log");
  const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 80;
  log.append(node);
  if (atEnd) log.scrollTop = log.scrollHeight;
  return node;
}

function setState(text, busy, ask) {
  $("state").textContent = text;
  $("spinner").hidden = !busy;
  $("status").classList.toggle("ask", !!ask);
  badge();
}

const working = () => (asking > 0 ? setState(t("gui.session.answer"), false, true) : setState(t("gui.working"), true));

function setIdle(on) {
  idle = on;
  $("reply-text").disabled = !on;
  $("reply-send").disabled = !on;
  if (on && view === "session") $("reply-text").focus();
  badge();
}

function setMode(mode) {
  for (const b of document.querySelectorAll("#mode button")) {
    b.classList.toggle("on", b.dataset.mode === mode);
    b.disabled = !live;
  }
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
function card(title, risk) {
  const root = el("div", "card");
  const h = el("h3", "", title);
  if (risk) h.append(el("span", "badge " + risk, t("gui.risk." + (risk === "dangerous" ? "danger" : risk === "normal" ? "normal" : "safe"))));
  root.append(h);
  const finish = (text) => {
    if (root.classList.contains("done")) return;
    root.classList.add("done");
    root.querySelectorAll("button, input").forEach((n) => (n.disabled = true));
    root.querySelector(".buttons")?.remove();
    root.querySelector(".row")?.remove();
    root.append(el("div", "answer", text));
    asking = Math.max(0, asking - 1);
    working();
  };
  asking++;
  setState(t("gui.session.answer"), false, true);
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
    if (decision === "all") setMode("auto");
  }, report);
  const buttons = el("div", "buttons");
  buttons.append(button(t("gui.approve"), "primary", answer("approve", t("gui.approve"))));
  if (!m.always) buttons.append(button(t("gui.approve_all"), "", answer("all", t("gui.approve_all"))));
  buttons.append(button(t("gui.deny"), "danger", answer("deny", t("gui.deny"))));
  c.root.append(buttons);
  add(c.root);
  if (view === "session") buttons.firstChild.focus();
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
    body.append(el("b", "", ch.label));
    if (ch.detail) body.append(el("div", "detail", ch.detail));
    row.append(input, body);
    if (ch.impact) row.append(el("span", "badge", ch.impact));
    if (ch.risk === "dangerous") row.append(el("span", "badge dangerous", t("gui.risk.danger")));
    c.root.append(row);
  }
  const picked = () => [...c.root.querySelectorAll("input:checked")].map((i) => i.value);
  const labels = (ids) => (m.choices ?? []).filter((ch) => ids.includes(ch.id)).map((ch) => ch.label).join(", ");
  const choose = button(t("gui.choose"), "primary", () => {
    const ids = picked();
    call("Choose", m.id, ids).then(() => c.finish(labels(ids)), report);
  });
  choose.disabled = true;
  c.root.addEventListener("change", () => (choose.disabled = picked().length === 0));
  const buttons = el("div", "buttons");
  buttons.append(choose, button(t("gui.choose_none"), "", () => call("Choose", m.id, []).then(() => c.finish(t("gui.choose_none")), report)));
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
  if (view === "session") input.focus();
}

function report(err) {
  add(el("div", "note bad", "! " + message(err)));
}

function handle(m) {
  if (m.stats) showStats(m.stats);
  switch (m.kind) {
    case "thinking":
      working();
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
      working();
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
      setMode(m.mode);
      break;
    case "idle":
      setIdle(true);
      setState(t("gui.session.message"), false);
      break;
    case "error":
      if (m.text?.trim()) add(el("div", "note bad", "! " + m.text.trim()));
      break;
    case "done":
      if (m.text) add(el("div", "note", m.text));
      break;
    case "closed":
      live = false;
      asking = 0;
      // Calls cut short by Stop never report an end.
      for (const e of tools.values()) e.mark.querySelector(".spinner")?.remove();
      setIdle(false);
      setState(t("gui.ended"), false);
      document.querySelectorAll(".card:not(.done)").forEach((c) => {
        c.classList.add("done");
        c.querySelectorAll("button, input").forEach((n) => (n.disabled = true));
      });
      // An ended session is only read: what acted on it goes away.
      $("stop").hidden = $("mode").hidden = $("reply").hidden = true;
      $("new").hidden = false;
      if (view === "home") showHome().catch(toastError);
      break;
  }
}

// ---------- history ----------

let runs = [];
let openRun = "";

const when = (iso) => new Date(iso).toLocaleString(info.lang, { dateStyle: "medium", timeStyle: "short" });

async function showHistory() {
  runs = await call("Runs");
  $("runs").replaceChildren(...runs.map((r) => {
    const b = el("button", "run" + (r.name === openRun ? " on" : ""));
    b.type = "button";
    b.dataset.run = r.name;
    b.append(el("b", "", when(r.time)), el("span", "", [r.ago, r.title].filter(Boolean).join(" · ")));
    return b;
  }));
  $("run-none-text").textContent = runs.length ? t("gui.history.pick") : t("logs.empty");
  if (!runs.some((r) => r.name === openRun)) {
    openRun = "";
    $("run-bar").hidden = $("run-text").hidden = true;
    $("run-none").hidden = false;
  }
}

// transcript draws a saved session: commands, their output and what omega said.
function transcript(text) {
  const out = [];
  let block = null; // consecutive output lines share one box
  const lines = text.replace(/\n+$/, "").split("\n");
  lines.forEach((line, i) => {
    if (line.startsWith("  ")) {
      // Calls that ran together write their output after another's end.
      if (!block) out.push((block = el("div", "l-out")));
      return block.append(line.slice(2) + "\n");
    }
    block = null;
    if (i === 0 && line.startsWith("omega run ")) return;
    if (!line.trim()) return;
    const exit = line.match(/^\[.* exit=(-?\d+) /);
    if (line.startsWith("$ ")) {
      out.push(el("div", "l-cmd", line));
      out.push((block = el("div", "l-out")));
    } else if (exit) out.push(el("div", "l-end" + (exit[1] === "0" ? "" : " bad"), exit[1] === "0" ? "✓" : "✗ exit " + exit[1]));
    else if (line.startsWith("? ")) out.push(el("div", "l-ask", line));
    else if (line.startsWith("! ")) out.push(el("div", "l-err", line));
    else if (line.startsWith("== ")) out.push(el("div", "l-sum", line.slice(3)));
    else out.push(el("div", "l-text", line));
  });
  return out.filter((n) => !n.classList.contains("l-out") || n.textContent);
}

async function showRun(name) {
  const r = runs.find((x) => x.name === name);
  if (!r) return;
  const text = await call("RunText", name);
  openRun = name;
  for (const b of document.querySelectorAll("#runs .run")) b.classList.toggle("on", b.dataset.run === name);
  $("run-when").textContent = when(r.time);
  $("run-title").textContent = r.title;
  $("run-copy").dataset.text = text;
  $("run-text").replaceChildren(...transcript(text));
  $("run-none").hidden = true;
  $("run-bar").hidden = $("run-text").hidden = false;
  $("run-text").scrollTop = 0;
}

// ---------- status ----------

async function showSystem() {
  const [steps, notes] = await Promise.all([call("Steps"), call("Notes")]);
  const nodes = [];
  let cat = "";
  for (const s of steps) {
    if (s.category !== cat) nodes.push(el("div", "steps-cat", (cat = s.category)));
    const row = el("div", "step" + (s.installed ? "" : " missing") + (s.skipped ? " off" : ""));
    const body = el("div", "body");
    body.append(el("div", "name", s.name), el("div", "id", s.id));
    row.append(body);
    if (!s.installed) row.append(el("span", "badge", t("gui.system.missing")));
    else if (s.dangerous) row.append(el("span", "badge dangerous", s.risk));
    else row.append(el("span", "badge", s.risk));
    const sw = el("label", "switch");
    const input = el("input");
    input.type = "checkbox";
    input.checked = !s.skipped;
    input.dataset.step = s.id;
    input.setAttribute("aria-label", s.name);
    sw.append(input, el("span"));
    row.append(sw);
    nodes.push(row);
  }
  $("steps").replaceChildren(...nodes);
  $("steps-count").textContent = t("gui.system.count")
    .replace("%d", steps.filter((s) => s.installed).length).replace("%d", steps.length);
  $("notes").textContent = notes || t("notes.empty");
  $("notes").classList.toggle("muted", !notes);
  $("notes-clear").hidden = !notes;
}

async function runCheck() {
  const b = $("check-run");
  b.disabled = true;
  b.textContent = t("gui.saving");
  try {
    const c = await call("Check");
    $("check").hidden = false;
    $("check-verdict").className = "verdict " + (c.ok ? "ok" : "bad");
    $("check-verdict").textContent = (c.ok ? "✓ " : "✗ ") + t(c.ok ? "gui.system.ok" : "gui.system.bad");
    $("check-lines").replaceChildren(...c.lines.map((l) => el("li", "", l)));
  } finally {
    b.disabled = false;
    b.textContent = t("gui.system.run");
  }
}

// ---------- settings ----------

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

async function loadProviders(keep) {
  providers = await call("Providers");
  $("providers").replaceChildren(...providers.map((p) => {
    const row = el("label", "opt");
    const input = el("input");
    input.type = "radio";
    input.name = "provider";
    input.value = p.name;
    input.checked = keep ? p.name === keep : p.current;
    const body = el("div", "body");
    body.append(el("b", "", p.label));
    row.append(input, body);
    if (p.status) row.append(el("span", "badge" + (p.hasKey || p.local ? " good" : ""), p.status));
    if (p.current) row.append(el("span", "badge", t("gui.in_use")));
    return row;
  }));
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
    startError = "";
    await refreshInfo();
    await loadProviders(p.name);
    loadModels().catch(() => {});
  } catch (err) {
    out.className = "result bad";
    out.textContent = message(err);
  }
  $("setup-save").disabled = false;
}

function options(select, pairs, value) {
  select.replaceChildren(...pairs.map(([v, label]) => {
    const o = el("option", "", label);
    o.value = v;
    return o;
  }));
  select.value = value;
}

// loadModels asks the provider what it offers. Until it answers, and when it
// cannot, the list holds the model in use.
async function loadModels() {
  const select = $("set-model");
  const current = (await call("Settings")).model;
  const label = (m) => [m.label, m.detail, m.badge].filter(Boolean).join(" · ");
  const fill = (list) => options(select, list.map((m) => [m.id, label(m)]), current);
  fill([{ id: "auto", label: t("gui.mode.auto") }, ...(current === "auto" ? [] : [{ id: current, label: current }])]);
  $("model-note").hidden = false;
  $("model-note").textContent = t("gui.set.model_load");
  try {
    fill(await call("Models"));
    $("model-note").hidden = true;
  } catch (err) {
    $("model-note").textContent = message(err);
  }
}

async function showSettings() {
  $("setup-result").hidden = true;
  const [s] = await Promise.all([call("Settings"), loadProviders()]);
  options($("set-language"), [["auto", t("gui.set.auto")], ...info.languages.map((l) => [l.code, l.name])], s.language);
  options($("set-approval"), ["dangerous", "all", "never"].map((v) => [v, t("gui.approval." + v)]), s.approval);
  options($("set-effort"), ["", "low", "medium", "high", "xhigh", "max"].map((v) => [v, t("gui.effort." + (v || "default"))]), s.effort);
  const times = ["15m", "30m", "45m", "1h", "2h"];
  if (!times.includes(s.timeout)) times.push(s.timeout);
  options($("set-timeout"), times.map((v) => [v, v.replace("m", " min").replace("h", " h")]), s.timeout);
  $("set-catalog").checked = s.catalog;
  $("set-path").textContent = s.path;
  loadModels().catch(toastError);
}

// setting saves one value and says so.
async function setting(key, value) {
  try {
    await call("SetSetting", key, value);
    if (key === "language") {
      info = await call("Info");
      applyTexts();
      await showSettings();
    }
    toast(t("gui.set.done") + " ✓");
  } catch (err) {
    toastError(err);
    showSettings().catch(() => {});
  }
}

// ---------- wiring ----------

function wire() {
  for (const n of document.querySelectorAll("i[data-icon]")) n.replaceWith(icon(n.dataset.icon));

  document.addEventListener("click", (e) => {
    const nav = e.target.closest?.("[data-view], [data-go]");
    if (nav) return go(nav.dataset.view ?? nav.dataset.go);
    const task = e.target.closest?.("#home [data-task]");
    if (task) return start(task.dataset.task, "");
    const run = e.target.closest?.("#runs .run");
    if (run) return showRun(run.dataset.run).catch(toastError);
    const mode = e.target.closest?.("#mode button");
    if (mode) return call("SetMode", mode.dataset.mode).then(setMode, toastError);
    // Links open in the browser, never in this window.
    const a = e.target.closest?.("a[href]");
    if (a) {
      e.preventDefault();
      const url = a.dataset.url ?? a.href;
      if (/^https?:\/\//.test(url ?? "")) wails.Browser.OpenURL(url);
    }
  });
  $("nav-provider").addEventListener("click", () => go("settings"));

  // Ctrl or ⌘ with 1 to 5 changes the view.
  document.addEventListener("keydown", (e) => {
    const n = Number(e.key);
    if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && n >= 1 && n <= views.length) {
      e.preventDefault();
      go(views[n - 1]);
    }
  });

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
  $("stop").addEventListener("click", () => call("Stop"));
  $("new").addEventListener("click", () => go("home"));

  $("run-copy").addEventListener("click", (e) => {
    wails.Clipboard.SetText(e.currentTarget.dataset.text ?? "").then(() => toast(t("gui.copied") + " ✓"), toastError);
  });

  $("check-run").addEventListener("click", () => runCheck().catch(toastError));
  $("steps").addEventListener("change", (e) => {
    const id = e.target.dataset.step;
    if (!id) return;
    call("SkipStep", id, !e.target.checked).then(() => {
      e.target.closest(".step").classList.toggle("off", !e.target.checked);
      toast(t("gui.set.done") + " ✓");
    }, (err) => {
      e.target.checked = !e.target.checked;
      toastError(err);
    });
  });
  $("notes-clear").addEventListener("click", () => call("ClearNotes").then(showSystem).catch(toastError));

  $("providers").addEventListener("change", () => {
    $("setup-result").hidden = true;
    showFields();
  });
  $("setup-form").addEventListener("submit", (e) => {
    e.preventDefault();
    saveSetup();
  });
  $("set-model").addEventListener("change", (e) => {
    call("SetModel", e.target.value).then(() => toast(t("gui.set.done") + " ✓"), toastError);
  });
  for (const [id, key] of [["set-language", "language"], ["set-approval", "approval"], ["set-effort", "effort"], ["set-timeout", "command_timeout"]]) {
    $(id).addEventListener("change", (e) => setting(key, e.target.value));
  }
  $("set-catalog").addEventListener("change", (e) => setting("model_catalog", e.target.checked ? "live" : "off"));

  wails.Events.On("omega", (ev) => handle(ev.data));
}

async function init() {
  info = await call("Info");
  wire();
  applyTexts();
  $("nav").hidden = $("views").hidden = false;
  // Without a provider there is nothing to start: open where one is chosen.
  go(info.ready || info.demo ? "home" : "settings");
}

init().catch((err) => {
  document.body.textContent = "omega: " + message(err);
});
