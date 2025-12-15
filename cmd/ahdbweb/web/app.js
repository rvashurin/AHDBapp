const state = {
  realms: [],
  selected: null,
  lastSeries: null,
  timer: null,
  chartMeta: null,
  histCache: new Map(),
  hoveredScanId: null,
  hoverTimer: null,
  histReqId: 0,
};

function $(id) {
  return document.getElementById(id);
}

function setStatus(msg, isError = false) {
  const el = $("status");
  el.textContent = msg || "";
  el.style.color = isError ? "var(--danger)" : "var(--muted)";
}

function setHistHint(msg, isError = false) {
  const el = $("histHint");
  if (!el) return;
  el.textContent = msg || "";
  el.style.color = isError ? "var(--danger)" : "var(--muted)";
}

function formatCopper(value) {
  const copper = Math.max(0, Math.round(Number(value) || 0));
  const gold = Math.floor(copper / 10000);
  const silver = Math.floor((copper % 10000) / 100);
  const c = copper % 100;
  return `${gold}g ${silver}s ${c}c`;
}

function formatTS(ts) {
  if (!ts) return "";
  return new Date(ts * 1000).toLocaleString();
}

async function fetchJSON(url) {
  const res = await fetch(url, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body && body.error) msg = body.error;
    } catch {
      // ignore
    }
    throw new Error(msg);
  }
  return res.json();
}

function renderRealmFactionOptions(realms) {
  const realmSel = $("realm");
  const factionSel = $("faction");
  realmSel.innerHTML = "";
  factionSel.innerHTML = "";

  const byRealm = new Map();
  for (const rf of realms) {
    if (!byRealm.has(rf.realm)) byRealm.set(rf.realm, new Set());
    byRealm.get(rf.realm).add(rf.faction);
  }

  const realmsSorted = Array.from(byRealm.keys()).sort();
  for (const realm of realmsSorted) {
    const opt = document.createElement("option");
    opt.value = realm;
    opt.textContent = realm;
    realmSel.appendChild(opt);
  }

  function updateFactions() {
    const realm = realmSel.value;
    const factions = Array.from(byRealm.get(realm) || []).sort();
    factionSel.innerHTML = "";
    for (const f of factions) {
      const opt = document.createElement("option");
      opt.value = f;
      opt.textContent = f;
      factionSel.appendChild(opt);
    }
  }

  realmSel.addEventListener("change", updateFactions);
  updateFactions();
}

function clearResults() {
  $("results").innerHTML = "";
}

function renderResults(items) {
  const results = $("results");
  results.innerHTML = "";
  for (const it of items) {
    const el = document.createElement("div");
    el.className = "result";
    el.innerHTML = `
      <div>
        <div class="resultName">${it.name}</div>
        <div class="mono">${it.id}</div>
      </div>
      <div class="mono">#${it.shortId}</div>
    `;
    el.addEventListener("click", () => {
      state.selected = it;
      $("selectedItem").textContent = `${it.name} (${it.id})`;
      clearResults();
      loadSeries();
    });
    results.appendChild(el);
  }
}

async function loadRealms() {
  setStatus("Loading realms…");
  const realms = await fetchJSON("/api/realms");
  state.realms = realms;
  renderRealmFactionOptions(realms);
  setStatus("");
}

async function searchItems(q) {
  const items = await fetchJSON(`/api/items?q=${encodeURIComponent(q)}`);
  return Array.isArray(items) ? items : [];
}

function scheduleSearch() {
  const q = $("search").value.trim();
  if (state.timer) clearTimeout(state.timer);
  if (q.length < 2) {
    clearResults();
    return;
  }
  state.timer = setTimeout(async () => {
    try {
      setStatus("Searching…");
      const items = await searchItems(q);
      renderResults(items);
      setStatus(items.length ? "" : "No matches");
    } catch (e) {
      setStatus(String(e.message || e), true);
    }
  }, 250);
}

function readControls() {
  const realm = $("realm").value;
  const faction = $("faction").value;
  const unit = $("unit").value;
  const days = Number($("days").value || 7);
  const maxPoints = Number($("maxPoints").value || 400);
  const trimPct = Number($("trimPct").value || 0);
  const metric = $("metric").value;
  const showStd = $("showStd").checked;
  return { realm, faction, unit, days, maxPoints, trimPct, metric, showStd };
}

async function loadSeries() {
  if (!state.selected) {
    setStatus("Select an item first.");
    return;
  }
  const c = readControls();
  const params = new URLSearchParams({
    itemId: state.selected.id,
    realm: c.realm,
    faction: c.faction,
    unit: c.unit,
    days: String(c.days),
    maxPoints: String(c.maxPoints),
    trimPct: String(c.trimPct),
  });

  try {
    setStatus("Loading series…");
    const series = await fetchJSON(`/api/series?${params.toString()}`);
    state.lastSeries = series;
    state.histCache = new Map();
    state.hoveredScanId = null;
    clearHistogram();
    draw(series.points, c.metric, c.showStd);
    renderTable(series.points);
    setStatus(series.points.length ? "" : "No data in that range");
  } catch (e) {
    setStatus(String(e.message || e), true);
  }
}

function renderTable(points) {
  const tbody = $("table").querySelector("tbody");
  tbody.innerHTML = "";
  for (const p of points) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td class="mono">${formatTS(p.ts)}</td>
      <td class="mono">${p.n}</td>
      <td class="mono">${formatCopper(p.mean)}</td>
      <td class="mono">${formatCopper(p.median)}</td>
      <td class="mono">${formatCopper(p.stddev)}</td>
    `;
    tbody.appendChild(tr);
  }
}

function clearHistogram(msg) {
  const canvas = $("hist");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  const w = canvas.width;
  const h = canvas.height;

  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = "rgba(0,0,0,0)";
  ctx.fillRect(0, 0, w, h);

  const text = msg || "Enable box plot and hover a snapshot to see the price distribution.";
  ctx.fillStyle = "rgba(255,255,255,0.65)";
  ctx.font = "14px ui-sans-serif, system-ui";
  ctx.fillText(text, 14, 28);
  setHistHint(text);
}

function drawHistogram(hist) {
  const canvas = $("hist");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  const w = canvas.width;
  const h = canvas.height;

  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = "rgba(0,0,0,0)";
  ctx.fillRect(0, 0, w, h);

  if (!hist || !hist.bins || !hist.bins.length) {
    clearHistogram("No histogram data");
    return;
  }

  const bins = hist.bins;
  const maxCount = Math.max(...bins.map((b) => b.count || 0), 1);

  const padL = 52;
  const padR = 14;
  const padT = 18;
  const padB = 34;
  const chartW = w - padL - padR;
  const chartH = h - padT - padB;
  const n = bins.length;
  const barW = chartW / n;

  function yScaleCount(c) {
    const clamped = Math.max(0, Math.min(maxCount, c));
    return padT + chartH - (clamped / maxCount) * chartH;
  }

  // Axes
  ctx.strokeStyle = "rgba(255,255,255,0.18)";
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(padL, padT);
  ctx.lineTo(padL, padT + chartH);
  ctx.lineTo(padL + chartW, padT + chartH);
  ctx.stroke();

  // Labels
  ctx.fillStyle = "rgba(255,255,255,0.62)";
  ctx.font = "12px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace";
  ctx.fillText(String(maxCount), 8, padT + 10);
  ctx.fillText("0", 8, padT + chartH);

  const fill = "rgba(125,211,252,0.35)";
  const stroke = "rgba(125,211,252,0.9)";

  for (let i = 0; i < n; i++) {
    const b = bins[i];
    const count = b.count || 0;
    const x0 = padL + i * barW;
    const x1 = x0 + barW * 0.9;
    const y0 = yScaleCount(count);
    const yBase = padT + chartH;
    ctx.fillStyle = fill;
    ctx.fillRect(x0, y0, Math.max(0.5, x1 - x0), yBase - y0);
    if (count > 0 && barW > 10) {
      ctx.strokeStyle = stroke;
      ctx.strokeRect(x0, y0, Math.max(0.5, x1 - x0), yBase - y0);
    }
  }

  const title = `Histogram @ ${formatTS(hist.ts)} — n=${hist.n}, trim=${hist.trimPct}%, unit=${hist.unit}`;
  setHistHint(title);

  // X labels
  ctx.fillStyle = "rgba(255,255,255,0.55)";
  ctx.font = "11px ui-sans-serif, system-ui";
  ctx.fillText(formatCopper(hist.min), padL, padT + chartH + 22);
  const maxLabel = formatCopper(hist.max);
  const tw = ctx.measureText(maxLabel).width;
  ctx.fillText(maxLabel, padL + chartW - tw, padT + chartH + 22);

  // Legend
  ctx.fillStyle = "rgba(255,255,255,0.72)";
  ctx.font = "12px ui-sans-serif, system-ui";
  ctx.fillText("Counts per price bucket", padL, 14);
}

function canvasEventToCanvasXY(canvas, evt) {
  const rect = canvas.getBoundingClientRect();
  const x = (evt.clientX - rect.left) * (canvas.width / rect.width);
  const y = (evt.clientY - rect.top) * (canvas.height / rect.height);
  return { x, y };
}

function hoveredBoxIndex(evt) {
  const meta = state.chartMeta;
  if (!meta || !meta.showStd || !meta.points || meta.points.length === 0) return null;
  const canvas = $("chart");
  if (!canvas) return null;
  const { x, y } = canvasEventToCanvasXY(canvas, evt);
  if (x < meta.padL || x > meta.padL + meta.chartW) return null;
  if (y < meta.padT || y > meta.padT + meta.chartH) return null;
  const boxW = meta.chartW / meta.n;
  const idx = Math.floor((x - meta.padL) / boxW);
  if (idx < 0 || idx >= meta.n) return null;
  return idx;
}

async function loadHistogramForScan(scanId) {
  if (!state.selected) return;
  const c = readControls();
  const unit = state.lastSeries?.unit || c.unit;
  const trimPct = typeof state.lastSeries?.trimPct === "number" ? state.lastSeries.trimPct : c.trimPct;
  const cacheKey = `${state.selected.id}|${scanId}|${unit}|${trimPct}`;
  const cached = state.histCache.get(cacheKey);
  if (cached) {
    drawHistogram(cached);
    return;
  }

  const params = new URLSearchParams({
    itemId: state.selected.id,
    scanId: String(scanId),
    unit,
    trimPct: String(trimPct),
    bins: "28",
  });

  const reqId = ++state.histReqId;
  setHistHint("Loading histogram…");
  try {
    const hist = await fetchJSON(`/api/histogram?${params.toString()}`);
    if (reqId !== state.histReqId) return;
    state.histCache.set(cacheKey, hist);
    drawHistogram(hist);
  } catch (e) {
    if (reqId !== state.histReqId) return;
    clearHistogram("Histogram error");
    setHistHint(String(e.message || e), true);
  }
}

function draw(points, metric, showStd) {
  const canvas = $("chart");
  const ctx = canvas.getContext("2d");
  const w = canvas.width;
  const h = canvas.height;

  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = "rgba(0,0,0,0)";
  ctx.fillRect(0, 0, w, h);

  if (!points || !points.length) {
    ctx.fillStyle = "rgba(255,255,255,0.65)";
    ctx.font = "14px ui-sans-serif, system-ui";
    ctx.fillText("No data", 14, 28);
    return;
  }

  const values = points.map((p) => (metric === "median" ? p.median : p.mean));
  const maxY = Math.max(
    ...points.map((p, i) => {
      if (showStd && typeof p.max === "number") return p.max;
      const v = values[i] || 0;
      const sd = p.stddev || 0;
      return showStd ? v + sd : v;
    }),
    1,
  );

  const padL = 52;
  const padR = 14;
  const padT = 16;
  const padB = 34;
  const chartW = w - padL - padR;
  const chartH = h - padT - padB;
  const n = points.length;

  state.chartMeta = { padL, padR, padT, padB, chartW, chartH, n, showStd, points };
  if (!showStd) {
    clearHistogram();
  }

  function yScale(v) {
    const clamped = Math.max(0, Math.min(maxY, v));
    return padT + chartH - (clamped / maxY) * chartH;
  }

  function xAt(i) {
    if (n <= 1) return padL + chartW / 2;
    if (showStd) {
      const boxW = chartW / n;
      return padL + i * boxW + boxW / 2;
    }
    return padL + (i / (n - 1)) * chartW;
  }

  // Axes
  ctx.strokeStyle = "rgba(255,255,255,0.18)";
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(padL, padT);
  ctx.lineTo(padL, padT + chartH);
  ctx.lineTo(padL + chartW, padT + chartH);
  ctx.stroke();

  // Y labels
  ctx.fillStyle = "rgba(255,255,255,0.62)";
  ctx.font = "12px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace";
  ctx.fillText(formatCopper(maxY), 8, padT + 10);
  ctx.fillText("0", 8, padT + chartH);

  if (showStd) {
    // Box plot: min/max whiskers + q1/q3 box + median line; mean dot.
    const boxW = chartW / n;
    const fill = "rgba(125,211,252,0.24)";
    const stroke = "rgba(125,211,252,0.9)";
    const meanDot = "rgba(52,211,153,0.9)";

    for (let i = 0; i < n; i++) {
      const p = points[i];
      const v = values[i] || 0;

      const minV = typeof p.min === "number" ? p.min : Math.max(0, v - (p.stddev || 0));
      const q1 = typeof p.q1 === "number" ? p.q1 : v;
      const med = typeof p.median === "number" ? p.median : v;
      const q3 = typeof p.q3 === "number" ? p.q3 : v;
      const maxV = typeof p.max === "number" ? p.max : v + (p.stddev || 0);

      const x0 = padL + i * boxW + boxW * 0.15;
      const x1 = padL + i * boxW + boxW * 0.85;
      const xc = (x0 + x1) / 2;

      const yMin = yScale(minV);
      const yMax = yScale(maxV);
      const yQ1 = yScale(q1);
      const yQ3 = yScale(q3);
      const yMed = yScale(med);

      ctx.strokeStyle = stroke;
      ctx.lineWidth = 1;

      // Whisker
      ctx.beginPath();
      ctx.moveTo(xc, yMax);
      ctx.lineTo(xc, yMin);
      ctx.moveTo(xc - 6, yMax);
      ctx.lineTo(xc + 6, yMax);
      ctx.moveTo(xc - 6, yMin);
      ctx.lineTo(xc + 6, yMin);
      ctx.stroke();

      // Box (q1..q3)
      const top = Math.min(yQ3, yQ1);
      const bottom = Math.max(yQ3, yQ1);
      ctx.fillStyle = fill;
      ctx.fillRect(x0, top, Math.max(1, x1 - x0), Math.max(1, bottom - top));
      ctx.strokeRect(x0, top, Math.max(1, x1 - x0), Math.max(1, bottom - top));

      // Median line
      ctx.beginPath();
      ctx.moveTo(x0, yMed);
      ctx.lineTo(x1, yMed);
      ctx.stroke();

      // Mean dot (optional)
      const yMean = yScale(typeof p.mean === "number" ? p.mean : v);
      ctx.fillStyle = meanDot;
      ctx.beginPath();
      ctx.arc(xc, yMean, 2.5, 0, Math.PI * 2);
      ctx.fill();
    }
  } else {
    // Simple line plot of the selected metric.
    const stroke = metric === "median" ? "rgba(52,211,153,0.95)" : "rgba(125,211,252,0.95)";
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    for (let i = 0; i < n; i++) {
      const x = xAt(i);
      const y = yScale(values[i] || 0);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    }
    ctx.stroke();

    // Points
    ctx.fillStyle = stroke;
    for (let i = 0; i < n; i++) {
      const x = xAt(i);
      const y = yScale(values[i] || 0);
      ctx.beginPath();
      ctx.arc(x, y, 2.2, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  // X labels (sparse)
  const ticks = 6;
  for (let t = 0; t <= ticks; t++) {
    const idx = Math.min(n - 1, Math.round((t / ticks) * (n - 1)));
    const x = xAt(idx);
    const label = new Date(points[idx].ts * 1000).toLocaleDateString();
    ctx.fillStyle = "rgba(255,255,255,0.55)";
    ctx.font = "11px ui-sans-serif, system-ui";
    ctx.fillText(label, x, padT + chartH + 22);
  }

  // Legend
  ctx.fillStyle = "rgba(255,255,255,0.72)";
  ctx.font = "12px ui-sans-serif, system-ui";
  if (showStd) {
    ctx.fillText("Box plot: q1–q3 (box), median (line), min/max (whiskers), mean (dot)", padL, 14);
  } else {
    ctx.fillText(`Line: ${metric} (${formatCopper(values[values.length - 1])} latest)`, padL, 14);
  }
}

function attachUI() {
  const metricSel = $("metric");
  const showStdCb = $("showStd");
  const syncMetricEnabled = () => {
    metricSel.disabled = showStdCb.checked;
  };
  syncMetricEnabled();
  clearHistogram();

  $("search").addEventListener("input", scheduleSearch);
  $("refresh").addEventListener("click", loadSeries);
  for (const id of ["realm", "faction", "unit", "days", "maxPoints", "trimPct", "metric", "showStd"]) {
    $(id).addEventListener("change", () => {
      syncMetricEnabled();
      if (state.selected && ["realm", "faction", "unit", "days", "maxPoints", "trimPct"].includes(id)) {
        loadSeries();
        return;
      }
      if (state.lastSeries) {
        const c = readControls();
        draw(state.lastSeries.points, c.metric, c.showStd);
        renderTable(state.lastSeries.points);
      }
    });
  }

  const chart = $("chart");
  chart.addEventListener("mousemove", (evt) => {
    const idx = hoveredBoxIndex(evt);
    if (idx === null) return;
    const p = state.chartMeta?.points?.[idx];
    if (!p || !p.scanId) return;
    if (p.scanId === state.hoveredScanId) return;
    state.hoveredScanId = p.scanId;
    if (state.hoverTimer) clearTimeout(state.hoverTimer);
    state.hoverTimer = setTimeout(() => loadHistogramForScan(p.scanId), 120);
  });
  chart.addEventListener("mouseleave", () => {
    state.hoveredScanId = null;
    if (state.hoverTimer) clearTimeout(state.hoverTimer);
    state.hoverTimer = null;
  });
}

async function main() {
  attachUI();
  try {
    await loadRealms();
  } catch (e) {
    setStatus(String(e.message || e), true);
  }
}

main();
