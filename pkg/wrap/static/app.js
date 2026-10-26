(() => {
  const STORAGE_KEY = "wrap_recent_links_v1";
  const THEME_KEY = "wrap_theme_v1";

  const htmlEl = document.documentElement;
  const themeToggleBtns = document.querySelectorAll(".theme-toggle-btn");
  const themeLabels = document.querySelectorAll(".theme-label");

  const siteHeader = document.getElementById("site-header");
  const mobileMenuBtn = document.getElementById("mobile-menu-btn");
  const mobileDrawer = document.getElementById("mobile-drawer");
  const iconMenuOpen = document.getElementById("icon-menu-open");
  const iconMenuClose = document.getElementById("icon-menu-close");

  // 1. Light / Dark Theme Toggle
  function applyTheme(theme) {
    const valid = theme === "dark" ? "dark" : "light";
    htmlEl.setAttribute("data-theme", valid);
    const nextLabel = valid === "dark" ? "Light" : "Dark";
    themeLabels.forEach((el) => {
      el.textContent = nextLabel;
    });
    try {
      localStorage.setItem(THEME_KEY, valid);
    } catch {
      // ignore
    }
  }

  try {
    const saved = localStorage.getItem(THEME_KEY);
    applyTheme(saved === "dark" ? "dark" : "light");
  } catch {
    applyTheme("light");
  }

  themeToggleBtns.forEach((btn) => {
    btn.addEventListener("click", () => {
      const current = htmlEl.getAttribute("data-theme") || "light";
      applyTheme(current === "dark" ? "light" : "dark");
    });
  });

  // 2. Mobile Hamburger Menu
  function setMobileMenu(open) {
    if (!mobileDrawer || !mobileMenuBtn) return;
    mobileDrawer.classList.toggle("hidden", !open);
    mobileMenuBtn.setAttribute("aria-expanded", open ? "true" : "false");
    if (iconMenuOpen && iconMenuClose) {
      iconMenuOpen.classList.toggle("hidden", open);
      iconMenuClose.classList.toggle("hidden", !open);
    }
  }

  if (mobileMenuBtn && mobileDrawer) {
    mobileMenuBtn.addEventListener("click", () => {
      const isHidden = mobileDrawer.classList.contains("hidden");
      setMobileMenu(isHidden);
    });

    document.addEventListener("pointerdown", (e) => {
      if (siteHeader && !siteHeader.contains(e.target)) {
        setMobileMenu(false);
      }
    });

    window.addEventListener("keydown", (e) => {
      if (e.key === "Escape") setMobileMenu(false);
    });
  }

  async function copyText(text, btn, originalLabel) {
    try {
      await navigator.clipboard.writeText(text);
      btn.textContent = "Copied";
      setTimeout(() => {
        btn.textContent = originalLabel;
      }, 1500);
    } catch {
      btn.textContent = "Error";
    }
  }

  // Copy buttons on /docs
  document.querySelectorAll(".copy-cmd-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const cmd = btn.getAttribute("data-cmd") || "";
      copyText(cmd, btn, "Copy");
    });
  });

  // Stop here if we are on /docs
  const shortenForm = document.getElementById("shorten-form");
  if (!shortenForm) return;

  const modeBtns = document.querySelectorAll("[data-mode]");
  const viewShorten = document.getElementById("view-shorten");
  const viewInspect = document.getElementById("view-inspect");

  const urlInput = document.getElementById("url-input");
  const aliasInput = document.getElementById("alias-input");
  const randomSlugBtn = document.getElementById("random-slug-btn");
  const ttlSelect = document.getElementById("ttl-select");
  const ttlOpts = document.querySelectorAll(".ttl-opt");
  const submitBtn = document.getElementById("submit-btn");
  const shortenError = document.getElementById("shorten-error");

  const resultTicket = document.getElementById("result-ticket");
  const resShortUrl = document.getElementById("res-short-url");
  const resTargetUrl = document.getElementById("res-target-url");
  const resStatelessUrl = document.getElementById("res-stateless-url");
  const resExpires = document.getElementById("res-expires");
  const openShortLink = document.getElementById("open-short-link");
  const copyShortBtn = document.getElementById("copy-short-btn");
  const copyStatelessBtn = document.getElementById("copy-stateless-btn");
  const viewStatsBtn = document.getElementById("view-stats-btn");
  const qrCanvas = document.getElementById("qr-canvas");
  const downloadQrBtn = document.getElementById("download-qr-btn");

  const ledgerSection = document.getElementById("ledger-section");
  const historyList = document.getElementById("history-list");
  const clearHistoryBtn = document.getElementById("clear-history-btn");

  const statsForm = document.getElementById("stats-form");
  const statsCodeInput = document.getElementById("stats-code-input");
  const statsError = document.getElementById("stats-error");
  const statsSheet = document.getElementById("stats-sheet");
  const statCodeTitle = document.getElementById("stat-code-title");
  const statTargetUrl = document.getElementById("stat-target-url");
  const statClicks = document.getElementById("stat-clicks");
  const statCreated = document.getElementById("stat-created");
  const statLastClicked = document.getElementById("stat-last-clicked");
  const statExpires = document.getElementById("stat-expires");
  const statReferrers = document.getElementById("stat-referrers");
  const refreshStatsBtn = document.getElementById("refresh-stats-btn");

  let activeCode = "";
  let activeShortUrl = "";
  let activeStatelessUrl = "";

  function switchMode(mode) {
    modeBtns.forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.mode === mode);
    });
    viewShorten.classList.toggle("active", mode === "shorten");
    viewInspect.classList.toggle("active", mode === "inspect");
    setMobileMenu(false);
  }

  modeBtns.forEach((btn) => {
    btn.addEventListener("click", () => switchMode(btn.dataset.mode));
  });

  function formatTimestamp(unixSecs) {
    if (!unixSecs || unixSecs <= 0) return "Never";
    return new Date(unixSecs * 1000).toLocaleString();
  }

  function showError(el, msg) {
    el.textContent = `ERROR: ${msg}`;
    el.classList.remove("hidden");
  }

  function hideError(el) {
    el.textContent = "";
    el.classList.add("hidden");
  }

  // Random Slug Generator
  randomSlugBtn.addEventListener("click", () => {
    const chars = "0123456789abcdefghijklmnopqrstuvwxyz";
    let slug = "";
    for (let i = 0; i < 6; i++) {
      slug += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    aliasInput.value = slug;
  });

  // Expiration Selector
  ttlOpts.forEach((opt) => {
    opt.addEventListener("click", () => {
      ttlSelect.value = opt.dataset.ttl;
      ttlOpts.forEach((o) => o.classList.toggle("active", o === opt));
    });
  });

  // Cobalt Blue Architectural QR Matrix
  function drawCodeMatrix(canvas, text) {
    const ctx = canvas.getContext("2d");
    const size = canvas.width;
    const cells = 21;
    const padding = 10;
    const gridW = size - padding * 2;
    const cellW = Math.floor(gridW / cells);

    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, size, size);

    let hash = 2166136261;
    const bytes = [];
    for (let i = 0; i < text.length; i++) {
      const code = text.charCodeAt(i);
      bytes.push(code);
      hash ^= code;
      hash = Math.imul(hash, 16777619);
    }

    function drawFinder(ox, oy) {
      ctx.fillStyle = "#1e3a8a";
      ctx.fillRect(padding + ox * cellW, padding + oy * cellW, 7 * cellW, 7 * cellW);
      ctx.fillStyle = "#ffffff";
      ctx.fillRect(padding + (ox + 1) * cellW, padding + (oy + 1) * cellW, 5 * cellW, 5 * cellW);
      ctx.fillStyle = "#1d4ed8";
      ctx.fillRect(padding + (ox + 2) * cellW, padding + (oy + 2) * cellW, 3 * cellW, 3 * cellW);
    }

    drawFinder(0, 0);
    drawFinder(cells - 7, 0);
    drawFinder(0, cells - 7);

    let seed = hash >>> 0;
    for (let r = 0; r < cells; r++) {
      for (let c = 0; c < cells; c++) {
        const inTopLeft = r < 8 && c < 8;
        const inTopRight = r < 8 && c >= cells - 8;
        const inBottomLeft = r >= cells - 8 && c < 8;
        if (inTopLeft || inTopRight || inBottomLeft) continue;

        seed = (Math.imul(seed, 1664525) + 1013904223 + (bytes[(r * cells + c) % bytes.length] || 0)) >>> 0;
        if ((seed & 3) === 0 || (seed & 5) === 0) {
          ctx.fillStyle = "#0f172a";
          ctx.fillRect(padding + c * cellW, padding + r * cellW, cellW, cellW);
        }
      }
    }
  }

  // Ledger History
  function loadHistory() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw ? JSON.parse(raw) : [];
    } catch {
      return [];
    }
  }

  function saveToHistory(entry) {
    const list = loadHistory().filter((item) => item.code !== entry.code);
    list.unshift(entry);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(list.slice(0, 12)));
    } catch {
      // ignore
    }
    renderHistory();
  }

  function renderHistory() {
    const list = loadHistory();
    if (!list.length) {
      ledgerSection.classList.add("hidden");
      return;
    }
    ledgerSection.classList.remove("hidden");
    historyList.innerHTML = "";

    list.forEach((item, index) => {
      const row = document.createElement("div");
      row.className = "ledger-row";

      const idx = document.createElement("span");
      idx.className = "ledger-idx";
      idx.textContent = String(index + 1).padStart(2, "0");

      const shortLink = document.createElement("a");
      shortLink.className = "ledger-short";
      shortLink.href = item.shortUrl;
      shortLink.target = "_blank";
      shortLink.rel = "noopener noreferrer";
      shortLink.textContent = item.shortUrl.replace(/^https?:\/\//, "");

      const dest = document.createElement("span");
      dest.className = "ledger-dest";
      dest.textContent = item.targetUrl;

      const actions = document.createElement("div");
      actions.className = "ledger-actions";

      const copyBtn = document.createElement("button");
      copyBtn.type = "button";
      copyBtn.className = "inline-trigger";
      copyBtn.textContent = "Copy";
      copyBtn.addEventListener("click", () => copyText(item.shortUrl, copyBtn, "Copy"));

      const inspectBtn = document.createElement("button");
      inspectBtn.type = "button";
      inspectBtn.className = "inline-trigger";
      inspectBtn.textContent = "Clicks →";
      inspectBtn.addEventListener("click", () => {
        statsCodeInput.value = item.code;
        switchMode("inspect");
        fetchStats(item.code);
      });

      actions.appendChild(copyBtn);
      actions.appendChild(inspectBtn);

      row.appendChild(idx);
      row.appendChild(shortLink);
      row.appendChild(dest);
      row.appendChild(actions);
      historyList.appendChild(row);
    });
  }

  clearHistoryBtn.addEventListener("click", () => {
    localStorage.removeItem(STORAGE_KEY);
    renderHistory();
  });

  // Shorten Submit
  shortenForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    hideError(shortenError);

    const url = urlInput.value.trim();
    const alias = aliasInput.value.trim();
    const ttl = ttlSelect.value;

    if (!url) {
      showError(shortenError, "Please enter a destination URL.");
      return;
    }

    submitBtn.disabled = true;
    submitBtn.textContent = "Wrapping...";

    try {
      const resp = await fetch("/api/shorten", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url, alias, ttl }),
      });
      const data = await resp.json();

      if (!resp.ok) {
        showError(shortenError, data.error || "Failed to wrap link.");
        return;
      }

      activeCode = data.code;
      activeShortUrl = data.shortUrl;
      activeStatelessUrl = data.statelessUrl;

      resShortUrl.textContent = data.shortUrl.replace(/^https?:\/\//, "");
      resShortUrl.href = data.shortUrl;
      resTargetUrl.textContent = data.targetUrl;
      resStatelessUrl.textContent = data.statelessUrl;
      resExpires.textContent = data.expiresAt
        ? `EXPIRES: ${formatTimestamp(data.expiresAt)}`
        : "EXPIRES: NEVER";
      openShortLink.href = data.shortUrl;

      drawCodeMatrix(qrCanvas, data.shortUrl);
      resultTicket.classList.remove("hidden");

      saveToHistory({
        code: data.code,
        shortUrl: data.shortUrl,
        targetUrl: data.targetUrl,
      });
    } catch {
      showError(shortenError, "Network error while contacting server.");
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = "Wrap URL →";
    }
  });

  copyShortBtn.addEventListener("click", () => {
    if (!activeShortUrl) return;
    copyText(activeShortUrl, copyShortBtn, "Copy Short Link");
  });

  copyStatelessBtn.addEventListener("click", () => {
    if (!activeStatelessUrl) return;
    copyText(activeStatelessUrl, copyStatelessBtn, "Copy Stateless /w/ Link");
  });

  viewStatsBtn.addEventListener("click", () => {
    if (!activeCode) return;
    statsCodeInput.value = activeCode;
    switchMode("inspect");
    fetchStats(activeCode);
  });

  downloadQrBtn.addEventListener("click", () => {
    const link = document.createElement("a");
    link.download = `wrap-${activeCode || "link"}.png`;
    link.href = qrCanvas.toDataURL("image/png");
    link.click();
  });

  // Telemetry Lookup
  async function fetchStats(rawInput) {
    hideError(statsError);
    statsSheet.classList.add("hidden");

    try {
      const resp = await fetch(`/api/stats?code=${encodeURIComponent(rawInput)}`);
      const data = await resp.json();

      if (!resp.ok) {
        showError(statsError, data.error || "Link not found.");
        return;
      }

      statCodeTitle.textContent = `/${data.code}`;
      statTargetUrl.textContent = data.url;
      statTargetUrl.href = data.url;
      statClicks.textContent = String(data.clicks || 0).padStart(2, "0");
      statCreated.textContent = data.createdAt ? formatTimestamp(data.createdAt) : "Stateless Token";
      statLastClicked.textContent = data.lastClicked ? formatTimestamp(data.lastClicked) : "No visits yet";
      statExpires.textContent = formatTimestamp(data.expiresAt);

      statReferrers.innerHTML = "";
      const refs = data.referrers || {};
      const keys = Object.keys(refs);

      if (!keys.length) {
        const empty = document.createElement("div");
        empty.className = "ref-line";
        empty.textContent = "No referrer hits recorded yet.";
        statReferrers.appendChild(empty);
      } else {
        keys
          .sort((a, b) => refs[b] - refs[a])
          .forEach((host) => {
            const row = document.createElement("div");
            row.className = "ref-line";
            const left = document.createElement("span");
            left.textContent = host;
            const right = document.createElement("span");
            right.textContent = `${refs[host]} visit${refs[host] === 1 ? "" : "s"}`;
            row.appendChild(left);
            row.appendChild(right);
            statReferrers.appendChild(row);
          });
      }

      statsSheet.classList.remove("hidden");
    } catch {
      showError(statsError, "Failed to load telemetry from server.");
    }
  }

  statsForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const val = statsCodeInput.value.trim();
    if (!val) {
      showError(statsError, "Enter a short code or URL to inspect.");
      return;
    }
    fetchStats(val);
  });

  refreshStatsBtn.addEventListener("click", () => {
    const val = statsCodeInput.value.trim() || activeCode;
    if (val) fetchStats(val);
  });

  renderHistory();
})();
