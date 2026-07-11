"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  api,
  getConfig,
  setConfig,
  type ApiConfig,
  type Stats,
  type SessionRow,
  type BattleEvent,
  type SystemService,
  type LlmStatus,
} from "@/lib/api";
import { translations, getLang, setLang, type Lang, type Dict } from "@/lib/i18n";

const pct = (x: number) => `${(x * 100).toFixed(0)}%`;
const CATEGORY_ORDER = ["Edge", "Detection", "Agents", "Runtime", "Data", "Ops"];
const GITHUB_URL = "https://github.com/Jest-Test-Team/Agentic-Defense-Matrix-ADM";
const TIMELINE_LIMIT = 1000;
const RED_TEAM_ATTACKS = [
  { id: "RT-001", attack: "Prompt Injection", technique: "Indirect injection via RAG context" },
  { id: "RT-002", attack: "Tool Chaining", technique: "read_secret → external_send chain" },
  { id: "RT-003", attack: "RAG Poisoning", technique: "Inject malicious URLs into knowledge base" },
  { id: "RT-004", attack: "Reverse Shell", technique: "bash -i >& /dev/tcp/... via tool call" },
  { id: "RT-005", attack: "Confused Deputy", technique: "Trick agent into privilege escalation" },
  { id: "RT-006", attack: "Token Theft", technique: "Replay captured JWT" },
  { id: "RT-007", attack: "Egress Exfiltration", technique: "DNS tunnel / HTTP POST to external" },
  { id: "RT-008", attack: "Container Escape", technique: "Mount host filesystem attempts" },
  { id: "RT-009", attack: "Rate Abuse", technique: "1000 req/min automated probing" },
  { id: "RT-010", attack: "State Drift", technique: "Modify agent context mid-session" },
  { id: "RT-011", attack: "LLM Supply Chain", technique: "Compromised Ollama model" },
  { id: "RT-012", attack: "Log Injection", technique: "Crafted payloads in user input" },
  { id: "RT-013", attack: "TOCTOU Race", technique: "Race condition in policy check" },
  { id: "RT-014", attack: "DNS Rebinding", technique: "Bypass egress filter via DNS" },
  { id: "RT-015", attack: "Privilege Escalation", technique: "Exploit Watchdog → root" },
  { id: "RT-016", attack: "Indirect Tool Output", technique: "Inject malicious instructions in tool output" },
  { id: "RT-017", attack: "Multi-Turn Context", technique: "Build trust then exploit across turns" },
  { id: "RT-018", attack: "Encoding Injection", technique: "Base64/hex encoded payloads" },
  { id: "RT-019", attack: "Multi-Language", technique: "Injection in multiple languages" },
  { id: "RT-020", attack: "Nested Injection", technique: "Nested system/user/assistant markers" },
  { id: "RT-021", attack: "Social Engineering", technique: "Fake admin/emergency commands" },
  { id: "RT-022", attack: "Payload Obfuscation", technique: "Variable splitting, concatenation" },
  { id: "RT-023", attack: "Supply Chain", technique: "Malicious package installation" },
  { id: "RT-024", attack: "Time-Based", technique: "Delayed trigger injection" },
  { id: "RT-025", attack: "Resource Exhaustion", technique: "Large payloads, concurrent requests" },
  { id: "RT-026", attack: "Memory Poisoning", technique: "Poison agent conversation memory" },
  { id: "RT-027", attack: "Cross-Session", technique: "Contaminate other sessions" },
  { id: "RT-028", attack: "Token Extraction", technique: "Extract API keys/tokens" },
  { id: "RT-029", attack: "Denial of Service", technique: "Excessive token generation" },
  { id: "RT-030", attack: "Side Channel", technique: "Data exfiltration via encoding" },
];

type Modal =
  | { kind: "svc"; svc: SystemService }
  | { kind: "sessions"; title: string; rows: SessionRow[] }
  | null;

export default function Page() {
  const [lang, setLangState] = useState<Lang>("en");
  const t = translations[lang];

  const [cfg, setCfg] = useState<ApiConfig | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [sessions, setSessions] = useState<SessionRow[]>([]);
  const [services, setServices] = useState<SystemService[]>([]);
  const [llm, setLlm] = useState<LlmStatus | null>(null);
  const [events, setEvents] = useState<BattleEvent[]>([]);
  const [connected, setConnected] = useState<boolean | null>(null);
  const [mixedContent, setMixedContent] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(true);
  const [modal, setModal] = useState<Modal>(null);
  const esRef = useRef<EventSource | null>(null);

  const landedRows = sessions.filter((s) => s.attack_outcome && s.attack_outcome !== "blocked");
  const remediatedRows = sessions.filter((s) => s.remediation_outcome);
  const blockedRows = sessions.filter((s) => s.attack_outcome === "blocked");
  const residualRows = landedRows.filter((s) => !s.remediation_outcome);
  const openSessions = (title: string, rows: SessionRow[]) => setModal({ kind: "sessions", title, rows });

  useEffect(() => {
    setCfg(getConfig());
    setLangState(getLang());
  }, []);

  const switchLang = (l: Lang) => {
    setLang(l);
    setLangState(l);
  };

  useEffect(() => {
    if (!cfg) return;
    if (typeof window !== "undefined" && window.location.protocol === "https:" && cfg.analysis.startsWith("http:")) {
      setMixedContent(true);
    }
  }, [cfg]);

  const refresh = useCallback(async () => {
    if (!cfg) return;
    try {
      setStats(await api.stats(cfg));
      setConnected(true);
    } catch {
      setConnected(false);
    }
    try {
      const tl = await api.timeline(cfg, TIMELINE_LIMIT);
      setSessions(tl.sessions ?? []);
    } catch {}
  }, [cfg]);

  const refreshSystem = useCallback(async () => {
    if (!cfg) return;
    try {
      const s = await api.system(cfg);
      setServices(s.services ?? []);
    } catch {
      setServices([]);
    }
    try {
      setLlm(await api.llm(cfg));
    } catch {
      setLlm(null);
    }
  }, [cfg]);

  useEffect(() => {
    if (!cfg) return;
    refresh();
    refreshSystem();
    const a = setInterval(refresh, 3000);
    const b = setInterval(refreshSystem, 8000);
    return () => {
      clearInterval(a);
      clearInterval(b);
    };
  }, [cfg, refresh, refreshSystem]);

  useEffect(() => {
    if (!cfg) return;
    try {
      const es = new EventSource(`${cfg.analysis}/api/stream`);
      esRef.current = es;
      es.onmessage = (e) => {
        try {
          setEvents((prev) => [JSON.parse(e.data) as BattleEvent, ...prev].slice(0, 120));
        } catch {}
      };
      es.onerror = () => {};
      return () => es.close();
    } catch {
      return;
    }
  }, [cfg]);

  return (
    <>
      <header className="top">
        <div className="top-inner">
          <h1 className="brand">
            ⚔️ ADM Battle Console
            <span className="sub">{t.subtitle}</span>
          </h1>
          <div className="langtoggle">
            <button className={lang === "en" ? "on" : ""} onClick={() => switchLang("en")}>EN</button>
            <button className={lang === "zh-Hant" ? "on" : ""} onClick={() => switchLang("zh-Hant")}>繁中</button>
          </div>
          <div className="conn">
            <span className={`dot ${connected === true ? "live" : connected === false ? "down" : ""}`} />
            {connected === true ? t.live : connected === false ? t.unreachable : t.connecting}
          </div>
        </div>
      </header>

      <div className="wrap">
        {mixedContent && <div className="banner">{t.mixedContent}</div>}

        <div className="about">
          <button className="about-h" onClick={() => setAboutOpen((v) => !v)}>
            <span>ℹ️ {t.aboutTitle}</span>
            <span className="chev">{aboutOpen ? "▾" : "▸"}</span>
          </button>
          {aboutOpen && (
            <div className="about-body">
              {t.aboutBody.map((p, i) => (
                <p key={i}>{p}</p>
              ))}
              <ul>
                <li><span className="tag red">RED</span> {t.aboutRed}</li>
                <li><span className="tag blue">BLUE</span> {t.aboutBlue}</li>
                <li><span className="tag green">GREEN</span> {t.aboutGreen}</li>
              </ul>
              <p className="muted">{t.aboutHowto}</p>
            </div>
          )}
        </div>

        <h2 className="section">{t.systemStatus}</h2>
        {services.length === 0 ? (
          <div className="status-grid">
            <div className="status-card"><div className="pill warn">…</div><div><div className="val">{t.checking}</div></div></div>
          </div>
        ) : (
          CATEGORY_ORDER.filter((cat) => services.some((s) => s.category === cat)).map((cat) => (
            <div key={cat} className="svc-group">
              <div className="svc-cat">{t.cat[cat] ?? cat}</div>
              <div className="status-grid">
                {services.filter((s) => s.category === cat).map((s) => (
                  <ServiceCard key={s.name} svc={s} t={t} onClick={() => setModal({ kind: "svc", svc: s })} />
                ))}
              </div>
            </div>
          ))
        )}

        <h2 className="section">{t.llmTitle}</h2>
        <div className="status-grid">
          {llm
            ? llm.providers.map((p) => <LlmCard key={p.role} p={p} t={t} />)
            : (
              <div className="status-card"><div className="pill warn">…</div><div><div className="val">{t.checking}</div></div></div>
            )}
        </div>

        <h2 className="section">{t.scoreboard}</h2>
        <div className="tiles">
          <Tile k={t.attacks} v={stats ? String(stats.attacks) : "–"} cls="red"
                onClick={() => openSessions(t.allSessions, sessions)} hint={t.clickHint} />
          <Tile k={t.blockRate} v={stats ? pct(stats.block_rate) : "–"} cls="blue"
                onClick={() => openSessions(t.blockedSessions, blockedRows)} hint={t.clickHint} />
          <Tile k={t.detectionRate} v={stats ? pct(stats.detection_rate) : "–"} cls="blue"
                onClick={() => openSessions(t.detectedSessions, sessions.filter((s) => s.attack_outcome === "detected"))} hint={t.clickHint} />
          <Tile k={t.landed} v={stats ? String(stats.landed) : "–"} cls="red"
                onClick={() => openSessions(t.landedSessions, landedRows)} hint={t.clickHint} />
          <Tile k={t.remediations} v={stats ? String(stats.remediations) : "–"} cls="good"
                onClick={() => openSessions(t.remediatedSessions, remediatedRows)} hint={t.clickHint} />
          <Tile k={t.mttr} v={stats ? (stats.mttr_seconds == null ? "–" : `${stats.mttr_seconds.toFixed(1)}s`) : "–"} cls="good"
                onClick={() => openSessions(t.mttrSessions, remediatedRows)} hint={t.clickHint} />
          <Tile k={t.residualRisk} v={stats ? String(stats.residual_risk) : "–"} cls="warn"
                onClick={() => openSessions(t.residualSessions, residualRows)} hint={t.clickHint} />
        </div>

        <div className="grid2" style={{ marginTop: 20 }}>
          <div>
            <h2 className="section">{t.liveFeed}</h2>
            <div className="panel tall">
              {events.length === 0 && (
                <div className="feed-row muted">{connected === false ? t.noStream : t.waitingEvents}</div>
              )}
              {events.map((ev, i) => (
                <EventRow key={ev.id ?? i} ev={ev} />
              ))}
            </div>
          </div>
          <div>
            <h2 className="section">{t.byTechnique}</h2>
            <div className="panel tall">
              <div className="legend">
                <span><span className="sw" style={{ background: "var(--blue)" }} />{t.blockedLegend}</span>
                <span><span className="sw" style={{ background: "var(--red)" }} />{t.landedLegend}</span>
              </div>
              {(stats?.by_technique ?? []).map((tech) => (
                <TechRow key={tech.technique} name={tech.technique} blocked={tech.blocked} landed={tech.landed} />
              ))}
              {!stats && <div className="feed-row muted">{t.loading}</div>}
            </div>
          </div>
        </div>

        <h2 className="section">{t.recentSessions}</h2>
        <div className="panel">
          <div className="feed-row muted" style={{ fontWeight: 600 }}>
            <span style={{ width: 90 }}>{t.colTechnique}</span>
            <span style={{ width: 90 }}>{t.colTarget}</span>
            <span>{t.colAttack}</span>
            <span className="out">{t.colRemediation}</span>
          </div>
          {sessions.slice(0, 20).map((s) => (
            <div className="feed-row" key={s.session_id}>
              <span className="tech" style={{ width: 90 }}>{s.technique}</span>
              <span className="muted" style={{ width: 90 }}>{s.target || "—"}</span>
              <span className={`out ${s.attack_outcome}`}>{s.attack_outcome}</span>
              <span className="out">
                <RemediationCell session={s} t={t} />
              </span>
            </div>
          ))}
          {sessions.length === 0 && <div className="feed-row muted">{t.noSessions}</div>}
        </div>

        <AttackMatrix t={t} />

        <Settings cfg={cfg} t={t} />

        <footer className="foot-note">
          <span>{t.footNote}</span>
          <a href={GITHUB_URL} target="_blank" rel="noopener noreferrer">{t.githubLink}</a>
        </footer>
      </div>

      {modal && <DetailModal modal={modal} t={t} onClose={() => setModal(null)} />}
    </>
  );
}

function DetailModal({ modal, t, onClose }: { modal: NonNullable<Modal>; t: Dict; onClose: () => void }) {
  return (
    <div className="modal-back" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <span>{modal.kind === "svc" ? modal.svc.name : modal.title}</span>
          <button className="modal-x" onClick={onClose}>✕</button>
        </div>
        <div className="modal-body">
          {modal.kind === "svc" ? (
            <div className="kv">
              <div className="kv-row"><span className="k">{t.statusLabel}</span>
                <span className={`v ${modal.svc.status === "up" ? "good-text" : modal.svc.status === "disabled" ? "warn-text" : "crit-text"}`}>
                  {modal.svc.status === "up" ? t.svcUp : modal.svc.status === "disabled" ? t.svcDisabled : t.svcDown}
                </span></div>
              <div className="kv-row"><span className="k">{t.technology}</span><span className="v">{modal.svc.tech}</span></div>
              <div className="kv-row"><span className="k">{t.category}</span><span className="v">{t.cat[modal.svc.category] ?? modal.svc.category}</span></div>
              <p className="modal-detail">{modal.svc.detail}</p>
              {modal.svc.hint && <div className="modal-hint">{modal.svc.hint}</div>}
            </div>
          ) : modal.rows.length === 0 ? (
            <div className="muted" style={{ padding: "8px 2px" }}>{t.noneYet}</div>
          ) : (
            <div>
              {modal.rows.map((s) => (
                <div className="feed-row" key={s.session_id}>
                  <span className="tech" style={{ width: 80 }}>{s.technique}</span>
                  <span className="muted" style={{ width: 90 }}>{s.target || "—"}</span>
                  <span className={`out ${s.attack_outcome}`}>{s.attack_outcome}</span>
                  <span className="out">
                    <RemediationCell session={s} t={t} />
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function ServiceCard({ svc, t, onClick }: { svc: SystemService; t: Dict; onClick: () => void }) {
  const cls = svc.status === "up" ? "good" : svc.status === "disabled" ? "warn" : "crit";
  const icon = svc.status === "up" ? "✓" : svc.status === "disabled" ? "○" : "✕";
  // A disabled component that carries a hint isn't broken — it's off by design.
  // Watchdog is host-only; others (otel) are one-flag opt-ins. Say so, so a
  // yellow card never reads as a fault.
  const isHostOnly = svc.name === "Endpoint Watchdog";
  const word =
    svc.status === "up"
      ? t.svcUp
      : svc.status === "disabled"
      ? svc.hint
        ? isHostOnly
          ? t.hostOnly
          : t.enableHint
        : t.svcDisabled
      : t.svcDown;
  return (
    <div className="status-card clickable" title={t.clickHint} onClick={onClick}>
      <div className={`pill ${cls}`}>{icon}</div>
      <div className="svc-meta">
        <div className="svc-name">{svc.name} <span className="svc-tech">{svc.tech}</span></div>
        <div className="svc-detail">{svc.detail}</div>
        <div className={`val ${cls}-text`}>{word}{svc.hint && <span className="svc-more"> · {t.clickHint} ›</span>}</div>
      </div>
    </div>
  );
}

function LlmCard({ p, t }: { p: LlmStatus["providers"][number]; t: Dict }) {
  // States: in-use (active+up) · stand-by (up, reachable, not active) ·
  // down (configured but unreachable) · not-configured (no key/url).
  const standby = p.status === "up" && !p.active;
  const cls = p.status === "up" ? "good" : p.status === "unconfigured" ? "warn" : "crit";
  const icon = p.active ? "▶" : p.status === "up" ? "✓" : p.status === "unconfigured" ? "○" : "✕";
  const word = p.active
    ? t.svcUp
    : standby
    ? t.llmStandby
    : p.status === "unconfigured"
    ? t.llmUnconfigured
    : t.svcDown;
  const roleLabel = p.role === "primary" ? t.llmPrimary : t.llmFallback;
  return (
    <div className="status-card" title={roleLabel}>
      <div className={`pill ${cls}`}>{icon}</div>
      <div className="svc-meta">
        <div className="svc-name">
          {p.name} <span className="svc-tech">{roleLabel}</span>
          {p.active && <span className="tag green" style={{ marginLeft: 6 }}>{t.llmActive}</span>}
          {standby && <span className="tag blue" style={{ marginLeft: 6 }}>{t.llmStandby}</span>}
        </div>
        <div className={`val ${cls}-text`}>{word}</div>
      </div>
    </div>
  );
}

function Tile({ k, v, cls, onClick, hint }: { k: string; v: string; cls?: string; onClick?: () => void; hint?: string }) {
  return (
    <div className={`tile ${onClick ? "clickable" : ""}`} onClick={onClick} title={onClick ? hint : undefined}>
      <div className="k">{k}</div>
      <div className={`v ${cls ?? ""}`}>{v}</div>
      {onClick && <div className="foot">{hint} ›</div>}
    </div>
  );
}

function TechRow({ name, blocked, landed }: { name: string; blocked: number; landed: number }) {
  const total = Math.max(1, blocked + landed);
  // A handful of landed hits against thousands blocked is a sub-pixel sliver —
  // floor any non-zero "landed" to a visible width so breaches never disappear.
  const landedPct = landed > 0 ? Math.max(6, (landed / total) * 100) : 0;
  return (
    <div className="tech-row">
      <span className="name">{name}</span>
      <span className="bar">
        <span className="blocked" style={{ width: `${100 - landedPct}%` }} />
        <span className="landed" style={{ width: `${landedPct}%` }} />
      </span>
      <span className="cnt">
        <b>{blocked}</b> ▏ <span className={landed > 0 ? "crit-text" : ""}>{landed}</span>
      </span>
    </div>
  );
}

function EventRow({ ev }: { ev: BattleEvent }) {
  const team = (ev.team || "?").toLowerCase();
  const tag = team === "red" ? "red" : team === "green" ? "green" : "blue";
  return (
    <div className="feed-row">
      <span className={`tag ${tag}`}>{(ev.team || "?").toUpperCase()}</span>
      <span className="tech">{ev.technique || ev.kind || ""}</span>
      <span className="detail">{ev.detail || ""}</span>
      <span className={`out ${ev.outcome || ""}`}>{ev.outcome || ""}</span>
    </div>
  );
}

function RemediationCell({ session, t }: { session: SessionRow; t: Dict }) {
  if (session.remediation_outcome) {
    return (
      <>
        {session.remediation_outcome}
        {session.mttr_seconds != null ? ` · ${session.mttr_seconds.toFixed(1)}s` : ""}
      </>
    );
  }
  return <span className="muted">{session.attack_outcome === "blocked" ? t.notNeeded : t.pending}</span>;
}

function AttackMatrix({ t }: { t: Dict }) {
  return (
    <>
      <h2 className="section">{t.redTeamMatrix}</h2>
      <div className="matrix-panel">
        <table className="attack-matrix">
          <thead>
            <tr>
              <th>{t.matrixId}</th>
              <th>{t.matrixAttack}</th>
              <th>{t.matrixTechnique}</th>
            </tr>
          </thead>
          <tbody>
            {RED_TEAM_ATTACKS.map((row) => (
              <tr key={row.id}>
                <td className="matrix-id">{row.id}</td>
                <td>{row.attack}</td>
                <td>{row.technique}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function Settings({ cfg, t }: { cfg: ApiConfig | null; t: Dict }) {
  const [analysis, setAnalysis] = useState("");
  const [gateway, setGateway] = useState("");
  useEffect(() => {
    if (cfg) {
      setAnalysis(cfg.analysis);
      setGateway(cfg.gateway);
    }
  }, [cfg]);
  return (
    <>
      <h2 className="section">{t.endpoint}</h2>
      <div className="settings">
        <input value={analysis} onChange={(e) => setAnalysis(e.target.value)} placeholder={t.analysisUrl} />
        <input value={gateway} onChange={(e) => setGateway(e.target.value)} placeholder={t.gatewayUrl} />
        <button
          onClick={() => {
            setConfig(analysis.trim(), gateway.trim());
            window.location.search = "";
          }}
        >
          {t.save}
        </button>
        <span className="muted">{t.orUse}</span>
      </div>
    </>
  );
}
