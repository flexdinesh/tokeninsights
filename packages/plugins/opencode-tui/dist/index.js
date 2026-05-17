// src/index.tsx
import { isAbsolute, join } from "node:path";
import Database from "better-sqlite3";
import { createMemo, createSignal } from "solid-js";
import { Fragment, jsx } from "@opentui/solid/jsx-runtime";
var STREAM_WINDOW_MS = 5e3;
var LIVE_STALE_MS = 1500;
var SINGLE_SAMPLE_MS = 1e3;
var BANNER_REFRESH_MS = 2e3;
var DEFAULT_DB_NAME = "tokeninsights.sqlite";
var DEFAULT_RETENTION_DAYS = 365;
function readStringOption(options, key) {
  const value = options?.[key];
  if (typeof value !== "string") return void 0;
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : void 0;
}
function readNumberOption(options, key, fallback) {
  const value = options?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}
function defaultDataPath() {
  const xdgDataHome = process.env.XDG_DATA_HOME?.trim();
  if (xdgDataHome && xdgDataHome.length > 0) return join(xdgDataHome, "tokeninsights");
  const home = process.env.HOME?.trim();
  if (home && home.length > 0) return join(home, ".local", "share", "tokeninsights");
  return join(process.cwd(), ".tokeninsights-data");
}
function storageConfig(options) {
  const dataPath = defaultDataPath();
  const configuredPath = readStringOption(options, "dbPath");
  const dbPath = configuredPath ? isAbsolute(configuredPath) ? configuredPath : join(dataPath, configuredPath) : join(dataPath, DEFAULT_DB_NAME);
  return {
    dbPath,
    retentionDays: readNumberOption(options, "retentionDays", DEFAULT_RETENTION_DAYS)
  };
}
function estimateStreamTokens(delta) {
  return Math.max(1, Math.ceil(Buffer.byteLength(delta, "utf8") / 5));
}
function formatRate(value, label) {
  if (!Number.isFinite(value) || value <= 0) return void 0;
  if (value >= 100) return `${Math.round(value)}${label === "TPS" ? " TPS" : ""}`;
  if (value >= 10) return `${value.toFixed(1)}${label === "TPS" ? " TPS" : ""}`;
  return `${value.toFixed(2)}${label === "TPS" ? " TPS" : ""}`;
}
function formatTtft(value) {
  if (!Number.isFinite(value) || value < 0) return void 0;
  return `${value.toFixed(1)}s`;
}
function activeDurationMs(samples, tailAt) {
  if (samples.length === 0) return 0;
  if (samples.length === 1) {
    const tailDuration = tailAt ? Math.max(0, tailAt - samples[0].at) : SINGLE_SAMPLE_MS;
    return Math.min(Math.max(tailDuration, 250), SINGLE_SAMPLE_MS);
  }
  let duration = 0;
  for (let i = 1; i < samples.length; i++) {
    duration += Math.max(0, samples[i].at - samples[i - 1].at);
  }
  if (tailAt) {
    duration += Math.max(0, tailAt - samples[samples.length - 1].at);
  }
  return Math.max(duration, SINGLE_SAMPLE_MS);
}
function numericOrNull(value) {
  return typeof value === "number" ? value : null;
}
function sessionAverageRow(value) {
  if (typeof value !== "object" || value === null) return void 0;
  return {
    throughputTokens: numericOrNull("throughput_tokens" in value ? value.throughput_tokens : void 0),
    durationMs: numericOrNull("duration_ms" in value ? value.duration_ms : void 0),
    avgTtftMs: numericOrNull("avg_ttft_ms" in value ? value.avg_ttft_ms : void 0)
  };
}
function querySessionAverages(dbPath, sessionID) {
  try {
    const db = new Database(dbPath, { readonly: true, fileMustExist: true });
    try {
      const stmt = db.prepare(`
        SELECT SUM(total_tokens) as throughput_tokens, SUM(duration_ms) as duration_ms, AVG(ttft_ms) as avg_ttft_ms
        FROM oc_tps_samples
        WHERE session_id = ? AND duration_ms > 0
      `);
      const row = sessionAverageRow(stmt.get(sessionID));
      if (!row) return void 0;
      const throughput = row.throughputTokens ?? 0;
      const duration = row.durationMs ?? 0;
      const avgTtftMs = row.avgTtftMs ?? 0;
      const avgTps = duration > 0 ? throughput / (duration / 1e3) : void 0;
      const avgTtft = avgTtftMs > 0 ? avgTtftMs / 1e3 : void 0;
      return { avgTps, avgTtft };
    } finally {
      db.close();
    }
  } catch {
    return void 0;
  }
}
function SessionPromptRight(props) {
  const sessionAverages = createMemo(() => {
    props.version();
    return querySessionAverages(props.dbPath, props.sessionID);
  });
  const sessionAverage = createMemo(() => {
    const averages = sessionAverages();
    if (!averages?.avgTps) return void 0;
    return formatRate(averages.avgTps, "AVG");
  });
  const sessionTtft = createMemo(() => {
    const averages = sessionAverages();
    if (!averages?.avgTtft) return void 0;
    return formatTtft(averages.avgTtft);
  });
  const liveTps = createMemo(() => {
    props.version();
    props.clock();
    const status = props.api.state.session.status(props.sessionID);
    if (status?.type === "idle") return void 0;
    const samples = props.streamSamplesBySession[props.sessionID] ?? [];
    if (samples.length === 0) return void 0;
    const now = Date.now();
    const relevant = samples.filter((sample) => now - sample.at <= STREAM_WINDOW_MS);
    if (relevant.length === 0) return void 0;
    const lastSample = relevant[relevant.length - 1];
    if (!lastSample || now - lastSample.at > LIVE_STALE_MS) return void 0;
    const total = relevant.reduce((sum, sample) => sum + sample.tokens, 0);
    const durationSeconds = activeDurationMs(relevant, now) / 1e3;
    if (durationSeconds <= 0) return void 0;
    return formatRate(total / durationSeconds, "AVG");
  });
  const text = createMemo(() => {
    const live = liveTps() ?? "-";
    const avg = sessionAverage() ?? "-";
    const ttft = sessionTtft() ?? "-";
    return `TPS ${live} | AVG ${avg} | TTFT ${ttft}`;
  });
  return /* @__PURE__ */ jsx(Fragment, { children: text() ? /* @__PURE__ */ jsx("text", { fg: props.api.theme.current.textMuted, children: text() }) : null });
}
var tui = async (api, options) => {
  const streamSamplesBySession = {};
  const [version, setVersion] = createSignal(0);
  const [clock, setClock] = createSignal(Date.now());
  const bump = () => setVersion((value) => value + 1);
  const dbConfig = storageConfig(options);
  const pruneSamples = (now = Date.now()) => {
    let changed = false;
    for (const [sessionID, samples] of Object.entries(streamSamplesBySession)) {
      const next = samples.filter((sample) => now - sample.at <= STREAM_WINDOW_MS);
      if (next.length !== samples.length) {
        changed = true;
        if (next.length > 0) streamSamplesBySession[sessionID] = next;
        else delete streamSamplesBySession[sessionID];
      }
    }
    if (changed) bump();
  };
  const clearLiveSamples = (sessionID) => {
    if (!streamSamplesBySession[sessionID]?.length) return;
    delete streamSamplesBySession[sessionID];
    bump();
  };
  const appendSample = (sessionID, _messageID, sample) => {
    const now = sample.at;
    streamSamplesBySession[sessionID] = [
      ...(streamSamplesBySession[sessionID] ?? []).filter((item) => now - item.at <= STREAM_WINDOW_MS),
      sample
    ];
    bump();
  };
  const onDelta = api.event.on("message.part.delta", (evt) => {
    if (evt.properties.field !== "text") return;
    const parts = api.state.part(evt.properties.messageID);
    const part = parts.find((item) => item.id === evt.properties.partID);
    if (!part) return;
    if (part.type !== "text" && part.type !== "reasoning") return;
    appendSample(evt.properties.sessionID, evt.properties.messageID, {
      at: Date.now(),
      tokens: estimateStreamTokens(evt.properties.delta)
    });
  });
  const onPart = api.event.on("message.part.updated", (evt) => {
    if (evt.properties.part.type !== "tool") return;
    if (evt.properties.part.state.status === "running" || evt.properties.part.state.status === "completed" || evt.properties.part.state.status === "error") {
      clearLiveSamples(evt.properties.sessionID);
    }
  });
  const timer = setInterval(() => {
    setClock(Date.now());
    pruneSamples();
  }, 1e3);
  const refreshTimer = setInterval(() => {
    bump();
  }, BANNER_REFRESH_MS);
  api.lifecycle.onDispose(() => {
    onDelta();
    onPart();
    clearInterval(timer);
    clearInterval(refreshTimer);
  });
  api.slots.register({
    slots: {
      session_prompt_right(_ctx, value) {
        return /* @__PURE__ */ jsx(
          SessionPromptRight,
          {
            api,
            sessionID: value.session_id,
            streamSamplesBySession,
            version,
            clock,
            dbPath: dbConfig.dbPath
          }
        );
      }
    }
  });
};
var plugin = {
  id: "oc-tokeninsights",
  tui
};
var index_default = plugin;
export {
  index_default as default
};
