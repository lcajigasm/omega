(() => {
  const root = document.documentElement;
  root.classList.remove("no-js");
  root.classList.add("js");
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  // The page's own strings, in its language, written on <body> by the build.
  const text = document.body.dataset;

  // Language menu: remember the choice, so the home page stops following the
  // browser's language, and close the menu like any other.
  document.querySelectorAll("a[data-lang]").forEach((a) => {
    a.addEventListener("click", () => {
      try { localStorage.setItem("omega-lang", a.dataset.lang); } catch { /* private window */ }
    });
  });
  const langMenu = document.querySelector("details.lang");
  if (langMenu) {
    document.addEventListener("click", (e) => { if (!langMenu.contains(e.target)) langMenu.open = false; });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") langMenu.open = false; });
  }

  // Copy a command, with explicit success and failure states. The status
  // line is the first .copy-status after the button's block.
  document.querySelectorAll(".copy-btn").forEach((btn) => {
    const scope = btn.closest(".install, .get");
    let status = scope ? scope.nextElementSibling : null;
    while (status && !status.classList.contains("copy-status")) status = status.nextElementSibling;
    const label = btn.querySelector(".copy-label");
    const original = label.textContent;
    btn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(btn.dataset.copy);
        label.textContent = text.copied || "Copied";
        if (status) {
          status.classList.remove("error");
          status.textContent = text.copyOk || "Command copied. Paste it into your terminal.";
        }
      } catch {
        if (status) {
          status.classList.add("error");
          status.textContent = text.copyFailed || "Copy failed. Select the command and copy it by hand.";
        }
      }
      setTimeout(() => { label.textContent = original; }, 2000);
    });
  });

  // Tabs: the install methods and the platforms. Without this script every
  // panel is simply shown one after another.
  document.querySelectorAll("[data-tabs]").forEach((group) => {
    const tabs = [...group.querySelectorAll('[role="tab"]')];
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
        if (step) {
          e.preventDefault();
          select(tabs[(i + step + tabs.length) % tabs.length], true);
        }
      });
    });
    select(tabs.find((t) => t.getAttribute("aria-selected") === "true") || tabs[0], false);
  });

  // A link to a section inside a tab opens that tab first.
  const openTabFor = (hash) => {
    const target = hash && document.querySelector(hash);
    const panel = target && target.closest('[role="tabpanel"]');
    if (panel && panel.hidden) document.getElementById(panel.getAttribute("aria-labelledby")).click();
  };
  window.addEventListener("hashchange", () => openTabFor(location.hash));
  openTabFor(location.hash);

  // Mobile menu.
  const toggle = document.querySelector(".menu-toggle");
  const menu = document.getElementById("mobile-menu");
  if (toggle && menu) {
    const setMenu = (open) => {
      toggle.setAttribute("aria-expanded", String(open));
      toggle.setAttribute("aria-label", open ? (text.menuClose || "Close menu") : (text.menuOpen || "Open menu"));
      document.body.style.overflow = open ? "hidden" : "";
      menu.hidden = !open;
    };
    toggle.addEventListener("click", () => setMenu(toggle.getAttribute("aria-expanded") !== "true"));
    menu.querySelectorAll("a").forEach((a) => a.addEventListener("click", () => setMenu(false)));
    document.addEventListener("keydown", (e) => { if (e.key === "Escape" && !menu.hidden) setMenu(false); });
    window.matchMedia("(min-width: 1041px)").addEventListener("change", (e) => { if (e.matches) setMenu(false); });
  }

  // Section reveals, once each.
  const revealObserver = new IntersectionObserver((entries, obs) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add("in");
        obs.unobserve(entry.target);
      }
    });
  }, { threshold: 0.06 });
  document.querySelectorAll(".reveal").forEach((el) => revealObserver.observe(el));

  // Hero terminal: lines appear in order.
  const terminal = document.querySelector(".hero .terminal");
  if (terminal) {
    terminal.querySelectorAll(".t-line").forEach((line, i) => line.style.setProperty("--d", reduced ? "0ms" : `${300 + i * 220}ms`));
    requestAnimationFrame(() => terminal.classList.add("play"));
  }

  // Mark the nav link of the section in view.
  const links = [...document.querySelectorAll(".nav-links a")];
  const spy = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      links.forEach((a) => a.classList.toggle("active", a.getAttribute("href") === `#${entry.target.id}`));
    });
  }, { rootMargin: "-45% 0px -50% 0px" });
  links.forEach((a) => {
    const target = document.querySelector(a.getAttribute("href"));
    if (target) spy.observe(target);
  });
})();
