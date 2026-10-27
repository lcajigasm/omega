(() => {
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // Copy install command, with explicit success and failure states.
  document.querySelectorAll(".copy-btn").forEach((btn) => {
    const status = btn.closest(".install").nextElementSibling;
    const label = btn.querySelector(".copy-label");
    const original = label.textContent;
    btn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(btn.dataset.copy);
        label.textContent = "Copied";
        status.classList.remove("error");
        status.textContent = "Install command copied. Paste it into your terminal.";
      } catch {
        status.classList.add("error");
        status.textContent = "Copy failed. Select the command and press ⌘C.";
      }
      setTimeout(() => { label.textContent = original; }, 2000);
    });
  });

  // Mobile menu: full height panel, staggered links (60ms each, 400ms cap).
  const toggle = document.querySelector(".menu-toggle");
  const menu = document.getElementById("mobile-menu");
  const setMenu = (open) => {
    toggle.setAttribute("aria-expanded", String(open));
    toggle.setAttribute("aria-label", open ? "Close menu" : "Open menu");
    document.body.style.overflow = open ? "hidden" : "";
    if (open) {
      menu.hidden = false;
      menu.querySelectorAll("li").forEach((li, i) => li.style.setProperty("--d", `${Math.min(i * 60, 400)}ms`));
      requestAnimationFrame(() => menu.classList.add("open"));
    } else {
      menu.classList.remove("open");
      menu.hidden = true;
    }
  };
  toggle.addEventListener("click", () => setMenu(toggle.getAttribute("aria-expanded") !== "true"));
  menu.querySelectorAll("a").forEach((a) => a.addEventListener("click", () => setMenu(false)));
  document.addEventListener("keydown", (e) => { if (e.key === "Escape" && !menu.hidden) setMenu(false); });

  // Section reveals, once each.
  const revealObserver = new IntersectionObserver((entries, obs) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add("in");
        obs.unobserve(entry.target);
      }
    });
  }, { threshold: 0.12 });
  document.querySelectorAll(".reveal").forEach((el) => revealObserver.observe(el));

  // Hero terminal: lines appear in order.
  const terminal = document.querySelector(".terminal");
  terminal.querySelectorAll(".t-line").forEach((line, i) => line.style.setProperty("--d", reduced ? "0ms" : `${300 + i * 260}ms`));
  requestAnimationFrame(() => terminal.classList.add("play"));

  // Tagline: each word lights up as it crosses a line 60% down the viewport.
  document.querySelectorAll("[data-words]").forEach((el) => {
    const words = el.textContent.trim().split(/\s+/);
    el.textContent = "";
    words.forEach((w, i) => {
      const span = document.createElement("span");
      span.className = "w";
      span.textContent = w;
      el.appendChild(span);
      if (i < words.length - 1) el.appendChild(document.createTextNode(" "));
    });
    const wordObserver = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        entry.target.classList.toggle("on", entry.isIntersecting || entry.boundingClientRect.top < 0);
      });
    }, { rootMargin: "0px 0px -40% 0px" });
    el.querySelectorAll(".w").forEach((w) => wordObserver.observe(w));
  });

  // Mark the nav link of the section in view.
  const links = [...document.querySelectorAll(".nav-links a")];
  const spy = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      links.forEach((a) => a.setAttribute("aria-current", String(a.getAttribute("href") === `#${entry.target.id}`)));
    });
  }, { rootMargin: "-45% 0px -50% 0px" });
  links.forEach((a) => {
    const target = document.querySelector(a.getAttribute("href"));
    if (target) spy.observe(target);
  });
})();
