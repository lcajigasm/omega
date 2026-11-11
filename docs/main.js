(() => {
  const root = document.documentElement;
  root.classList.remove("no-js");
  root.classList.add("js");
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const $ = (sel, scope = document) => scope.querySelector(sel);
  const $$ = (sel, scope = document) => [...scope.querySelectorAll(sel)];

  // The page's own strings, in its language, written by the build.
  let text = {};
  try { text = JSON.parse($("#strings").textContent); } catch { /* the privacy page has none */ }
  const fill = (s, ...n) => n.reduce((out, v) => out.replace("%d", v), s || "");
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

  // The system the visitor is on, named as Go names it.
  const os = (() => {
    const p = ((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || navigator.userAgent || "").toLowerCase();
    if (p.includes("win")) return "windows";
    if (p.includes("mac") || p.includes("iphone") || p.includes("ipad")) return "darwin";
    if (p.includes("linux") || p.includes("x11") || p.includes("cros")) return "linux";
    return "";
  })();

  // Theme: the system's until the visitor picks one.
  const themeBtn = $("#theme");
  if (themeBtn) {
    themeBtn.addEventListener("click", () => {
      const now = root.dataset.theme || (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
      const next = now === "dark" ? "light" : "dark";
      root.dataset.theme = next;
      try { localStorage.setItem("omega-theme", next); } catch { /* private window */ }
    });
  }

  // Language menu: remember the choice, so the home page stops following the
  // browser's language, and close the menu like any other.
  $$("a[data-lang]").forEach((a) => {
    a.addEventListener("click", () => {
      try { localStorage.setItem("omega-lang", a.dataset.lang); } catch { /* private window */ }
    });
  });
  const langMenu = $("details.lang");
  if (langMenu) {
    document.addEventListener("click", (e) => { if (!langMenu.contains(e.target)) langMenu.open = false; });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") langMenu.open = false; });
  }

  // Copy a command, with explicit success and failure states. The status
  // line is the first .copy-status after the button's block.
  $$(".copy-btn").forEach((btn) => {
    const scope = btn.closest(".install, .get");
    let status = scope ? scope.nextElementSibling : null;
    while (status && !status.classList.contains("copy-status")) status = status.nextElementSibling;
    const label = $(".copy-label", btn);
    const original = label.textContent;
    btn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(btn.dataset.copy);
        label.textContent = text.copied || "Copied";
        if (status) { status.classList.remove("error"); status.textContent = text.copyOk || ""; }
      } catch {
        if (status) { status.classList.add("error"); status.textContent = text.copyFailed || ""; }
      }
      setTimeout(() => { label.textContent = original; }, 2000);
    });
  });

  // Tabs. Without this script every panel is shown one after another. A
  // group opens on the visitor's own system when it has a tab for it.
  $$("[data-tabs]").forEach((group) => {
    const tabs = $$('[role="tab"]', group).filter((t) => t.closest("[data-tabs]") === group);
    const select = (tab, focus) => {
      tabs.forEach((t) => {
        const on = t === tab;
        t.setAttribute("aria-selected", String(on));
        t.tabIndex = on ? 0 : -1;
        document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
      });
      if (focus) tab.focus();
    };
    tabs.forEach((tab, i) => {
      tab.addEventListener("click", () => select(tab, false));
      tab.addEventListener("keydown", (e) => {
        const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
        if (step) { e.preventDefault(); select(tabs[(i + step + tabs.length) % tabs.length], true); }
      });
    });
    select(tabs.find((t) => os && t.dataset.os === os) || tabs.find((t) => t.getAttribute("aria-selected") === "true") || tabs[0], false);
  });

  // A link to something inside a tab opens that tab first.
  const openTabFor = (hash) => {
    let target = null;
    try { target = hash && $(hash); } catch { /* not a selector */ }
    const panel = target && target.closest('[role="tabpanel"]');
    if (panel && panel.hidden) {
      document.getElementById(panel.getAttribute("aria-labelledby")).click();
      target.scrollIntoView();
    }
  };
  window.addEventListener("hashchange", () => openTabFor(location.hash));
  openTabFor(location.hash);

  // The index: a rail on a wide screen, a sheet on a narrow one.
  const toggle = $(".menu-toggle");
  const rail = $("#index");
  if (toggle && rail) {
    const setMenu = (open) => {
      toggle.setAttribute("aria-expanded", String(open));
      toggle.setAttribute("aria-label", open ? text.menuClose : text.menuOpen);
      rail.classList.toggle("open", open);
      document.body.style.overflow = open ? "hidden" : "";
    };
    toggle.addEventListener("click", () => setMenu(toggle.getAttribute("aria-expanded") !== "true"));
    $$("a", rail).forEach((a) => a.addEventListener("click", () => setMenu(false)));
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") setMenu(false); });
    window.matchMedia("(min-width: 1121px)").addEventListener("change", (e) => { if (e.matches) setMenu(false); });

    const links = $$("ol a", rail);
    const spy = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        links.forEach((a) => a.classList.toggle("active", a.getAttribute("href") === `#${entry.target.id}`));
      });
    }, { rootMargin: "-30% 0px -65% 0px" });
    links.forEach((a) => { const t = $(a.getAttribute("href")); if (t) spy.observe(t); });
    const hero = $("#top");
    if (hero) new IntersectionObserver(([e]) => { if (e.isIntersecting) links.forEach((a) => a.classList.remove("active")); }, { rootMargin: "-30% 0px -65% 0px" }).observe(hero);
  }

  // The tour: one camera over captures of a real session. Each step names a
  // frame and the rectangle of it, in the capture's own pixels, to look at.
  $$("[data-tour]").forEach((tour) => {
    const stage = $(".tour-stage", tour);
    const mark = $(".tour-mark", tour);
    const imgs = $$(".tour-img", tour);
    const steps = $$(".tour-steps button", tour);
    const W = 1280, H = 820;
    let current = 0, timer = null, playing = !reduced;

    const show = (i) => {
      current = i;
      const step = steps[i];
      const [x, y, w, h] = step.dataset.rect.split(",").map(Number);
      const cw = stage.clientWidth, ch = stage.clientHeight;
      const pad = 34;
      const scale = Math.max(0.6, Math.min(1.25, cw / (w + pad * 2), ch / (h + pad * 2)));
      // Centre the rectangle; when it is wider than the stage, keep its left
      // edge, where the lines of a terminal start. Never show past an edge.
      let tx = w * scale > cw - 24 ? 12 - x * scale : cw / 2 - (x + w / 2) * scale;
      let ty = ch / 2 - (y + h / 2) * scale;
      tx = W * scale <= cw ? (cw - W * scale) / 2 : Math.min(0, Math.max(cw - W * scale, tx));
      ty = H * scale <= ch ? (ch - H * scale) / 2 : Math.min(0, Math.max(ch - H * scale, ty));
      imgs.forEach((img) => {
        img.classList.toggle("is-on", img.dataset.frame === step.dataset.frame);
        img.style.transform = `translate(${tx}px, ${ty}px) scale(${scale})`;
      });
      const m = 7;
      mark.style.left = `${tx + (x - m) * scale}px`;
      mark.style.top = `${ty + (y - m) * scale}px`;
      mark.style.width = `${Math.min((w + m * 2) * scale, cw - (tx + (x - m) * scale) - 6)}px`;
      mark.style.height = `${(h + m * 2) * scale}px`;
      steps.forEach((s, n) => s.setAttribute("aria-current", String(n === i)));
    };
    const play = () => {
      clearInterval(timer);
      tour.classList.toggle("is-playing", playing);
      if (playing) timer = setInterval(() => show((current + 1) % steps.length), 6000);
    };
    steps.forEach((s, i) => s.addEventListener("click", () => { playing = false; play(); show(i); }));
    new ResizeObserver(() => show(current)).observe(stage);
    new IntersectionObserver(([e]) => {
      if (!playing) return;
      if (e.isIntersecting) play(); else { clearInterval(timer); tour.classList.remove("is-playing"); }
    }, { threshold: 0.3 }).observe(tour);
    show(0);
  });

  // The command index filters as you type, like `omega menu`.
  const cmdFilter = $("#cmd-filter");
  if (cmdFilter) {
    const groups = $$(".cmd-group");
    const items = $$(".cmd-group li");
    const count = $("#cmd-count");
    const norm = (s) => s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
    const apply = () => {
      const words = norm(cmdFilter.value).split(/\s+/).filter(Boolean);
      let shown = 0;
      items.forEach((li) => {
        const hay = norm(li.textContent + " " + li.closest(".cmd-group").querySelector("h3").textContent);
        const on = words.every((w) => hay.includes(w));
        li.hidden = !on;
        if (on) shown++;
      });
      groups.forEach((g) => { g.hidden = !$$("li", g).some((li) => !li.hidden); });
      count.textContent = shown === 0 ? text.cmdNone : fill(text.cmdCount, shown, items.length);
    };
    cmdFilter.addEventListener("input", apply);
    apply();
  }

  // The catalogue: every step of the three systems, from the file the build
  // wrote out of the binary's own table.
  const cat = $("#cat");
  if (cat) {
    const rows = $("#cat-rows"), count = $("#cat-count"), filter = $("#cat-filter");
    const osBtns = $$("[data-os]", cat), catBtns = $$("[data-cat]", cat);
    const state = { os: osBtns.some((b) => b.dataset.os === os) ? os : "darwin", cat: "", q: "" };
    let data = null;
    const press = (btns, on) => btns.forEach((b) => b.setAttribute("aria-pressed", String(on(b))));
    const render = () => {
      press(osBtns, (b) => b.dataset.os === state.os);
      press(catBtns, (b) => b.dataset.cat === state.cat);
      if (!data) return;
      const all = data[state.os] || [];
      const words = state.q.toLowerCase().split(/\s+/).filter(Boolean);
      const list = all.filter((s) => (!state.cat || s.cat === state.cat) &&
        words.every((w) => (s.id + " " + s.name + " " + s.cmd).toLowerCase().includes(w)));
      rows.innerHTML = list.length ? list.map((s) =>
        `<tr title="${esc(text.catMore)}"><td class="c-id"><code>${esc(s.id)}</code></td><td class="c-name">${esc(s.name)}</td>` +
        `<td class="c-cmd"><code>${esc(s.cmd)}</code></td><td class="c-risk"><span class="badge" data-risk="${esc(s.risk)}">${esc(text.lvl[s.risk] || s.risk)}</span></td></tr>`).join("")
        : `<tr class="cat-empty"><td colspan="4">${esc(text.catNone)}</td></tr>`;
      count.textContent = fill(text.catCount, list.length, all.length);
      rows.parentElement.parentElement.scrollTop = 0;
    };
    const load = () => {
      if (load.started) return;
      load.started = true;
      fetch(cat.dataset.src).then((r) => r.json()).then((d) => { data = d; render(); }).catch(() => { load.started = false; });
    };
    osBtns.forEach((b) => b.addEventListener("click", () => { state.os = b.dataset.os; load(); render(); }));
    catBtns.forEach((b) => b.addEventListener("click", () => { state.cat = b.dataset.cat; load(); render(); }));
    filter.addEventListener("input", () => { state.q = filter.value; load(); render(); });
    rows.addEventListener("click", (e) => { const tr = e.target.closest("tr"); if (tr && !tr.classList.contains("cat-empty")) tr.classList.toggle("open"); });
    render();
    new IntersectionObserver(([e], obs) => { if (e.isIntersecting) { load(); obs.disconnect(); } }, { rootMargin: "800px 0px" }).observe(cat);
  }

  // The classifier. The examples carry the answer omega's code gave when the
  // page was built; typing loads that same code, compiled to WebAssembly,
  // and from then on every answer is computed here, in the browser.
  const clf = $("#clf");
  if (clf) {
    const input = $("#clf-in"), out = $("#clf-out"), status = $("#clf-status");
    const badge = $(".badge", out), verdict = $(".clf-verdict", out);
    let ready = null;
    const paint = (risk) => {
      out.dataset.risk = risk;
      badge.dataset.risk = risk;
      badge.textContent = text.lvl[risk] || risk;
      verdict.textContent = text.verdict[risk] || "";
    };
    const classify = () => {
      if (typeof window.omegaClassify !== "function") return false;
      const cmd = input.value.trim();
      if (!cmd) { out.hidden = true; return true; }
      out.hidden = false;
      const r = window.omegaClassify(cmd);
      paint(r.risk !== "blocked" && r.always ? "always" : r.risk);
      return true;
    };
    const script = (src) => new Promise((ok, fail) => {
      const s = document.createElement("script");
      s.src = src; s.onload = ok; s.onerror = fail;
      document.head.appendChild(s);
    });
    const load = () => ready || (ready = (async () => {
      status.textContent = text.clfLoading;
      await script(clf.dataset.exec);
      const go = new window.Go();
      let result;
      try {
        result = await WebAssembly.instantiateStreaming(fetch(clf.dataset.wasm), go.importObject);
      } catch {
        // A server that does not send application/wasm.
        result = await WebAssembly.instantiate(await (await fetch(clf.dataset.wasm)).arrayBuffer(), go.importObject);
      }
      go.run(result.instance);
      status.textContent = text.clfLive;
      status.classList.add("is-live");
    })().catch(() => { ready = null; status.textContent = text.clfFailed; throw new Error("classifier"); }));
    const live = () => load().then(classify, () => {});
    input.addEventListener("focus", () => { load().catch(() => {}); });
    input.addEventListener("input", () => { if (!classify()) live(); });
    $$(".ex", clf).forEach((b) => b.addEventListener("click", () => {
      input.value = b.dataset.cmd;
      out.hidden = false;
      // The build's answer now, the live one as soon as the classifier is in.
      paint(b.dataset.risk);
      live();
    }));
  }

  // Captures: one large, the rest as thumbnails. Without this script they
  // are simply shown in a grid.
  let lettered = 0;
  $$("[data-plates]").forEach((box) => {
    const figs = $$("figure", box);
    const thumbs = document.createElement("div");
    thumbs.className = "thumbs";
    const section = box.closest("section");
    const no = section ? ($(".sec-no span", section) || {}).textContent : "";
    const select = (i) => {
      figs.forEach((f, k) => f.classList.toggle("is-on", k === i));
      $$("button", thumbs).forEach((b, k) => b.setAttribute("aria-pressed", String(k === i)));
    };
    figs.forEach((fig, i) => {
      const img = $("img", fig), cap = $("figcaption", fig);
      const letter = String.fromCharCode(97 + lettered++);
      const label = `${text.fig || "Fig."} ${Number(no) || ""}${letter}`;
      cap.insertAdjacentHTML("afterbegin", `<b>${esc(label)}</b> `);
      const b = document.createElement("button");
      b.type = "button";
      b.setAttribute("aria-label", `${label}: ${cap.textContent.replace(label, "").trim()}`);
      b.innerHTML = `<img src="${esc(img.getAttribute("src"))}" alt="" loading="lazy" width="${img.width}" height="${img.height}"><span>${letter}</span>`;
      b.addEventListener("click", () => select(i));
      thumbs.appendChild(b);
    });
    box.classList.add("is-live");
    box.appendChild(thumbs);
    select(0);
  });

  // The recorded session: the player and the recording load when it opens.
  $$("details[data-cast]").forEach((d) => {
    d.addEventListener("toggle", () => {
      if (!d.open || d.dataset.loaded) return;
      d.dataset.loaded = "1";
      const base = "https://cdn.jsdelivr.net/npm/asciinema-player@3.8.2/dist/bundle/asciinema-player";
      const css = document.createElement("link");
      css.rel = "stylesheet"; css.href = `${base}.css`;
      document.head.appendChild(css);
      const s = document.createElement("script");
      s.src = `${base}.min.js`;
      s.onload = () => window.AsciinemaPlayer.create(d.dataset.cast, $(".cast-body", d), { cols: 100, rows: 32, autoPlay: true, fit: "width", idleTimeLimit: 2 });
      document.head.appendChild(s);
    });
  });
})();
