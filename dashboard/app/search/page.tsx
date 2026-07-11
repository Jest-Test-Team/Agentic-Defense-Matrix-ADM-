"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, getConfig, type ApiConfig, type SearchResult } from "@/lib/api";
import { translations, getLang, setLang, type Lang } from "@/lib/i18n";

const EXAMPLES = [
  "reverse shell",
  "team:red AND outcome:allowed",
  "technique:RT-028",
  "container escape",
  "*",
];

export default function SearchPage() {
  const [lang, setLangState] = useState<Lang>("en");
  const t = translations[lang];
  const [cfg, setCfg] = useState<ApiConfig | null>(null);
  const [q, setQ] = useState("*");
  const [res, setRes] = useState<SearchResult | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setCfg(getConfig());
    setLangState(getLang());
  }, []);

  const run = useCallback(
    async (query: string) => {
      if (!cfg) return;
      setLoading(true);
      try {
        setRes(await api.search(cfg, query));
      } catch {
        setRes({ total: 0, hits: [], error: "unreachable" });
      } finally {
        setLoading(false);
      }
    },
    [cfg]
  );

  // Auto-run once config is ready.
  useEffect(() => {
    if (cfg) run("*");
  }, [cfg, run]);

  const switchLang = (l: Lang) => {
    setLang(l);
    setLangState(l);
  };

  return (
    <>
      <header className="top">
        <div className="top-inner">
          <h1 className="brand">
            🔎 {t.searchTitle}
          </h1>
          <div className="langtoggle">
            <button className={lang === "en" ? "on" : ""} onClick={() => switchLang("en")}>EN</button>
            <button className={lang === "zh-Hant" ? "on" : ""} onClick={() => switchLang("zh-Hant")}>繁中</button>
          </div>
          <div className="conn">
            <Link href="/">{t.backToConsole}</Link>
          </div>
        </div>
      </header>

      <div className="wrap">
        <div className="about" style={{ marginTop: 20 }}>
          <div className="about-body" style={{ borderTop: "none" }}>
            {t.searchIntro.map((p, i) => (
              <p key={i}>{p}</p>
            ))}
          </div>
        </div>

        <form
          className="searchbar"
          onSubmit={(e) => {
            e.preventDefault();
            run(q);
          }}
        >
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t.searchPlaceholder} aria-label="query" />
          <button type="submit">{t.searchButton}</button>
        </form>

        <div className="examples">
          <span className="muted">{t.searchExamplesLabel}</span>
          {EXAMPLES.map((ex) => (
            <button key={ex} className="chip" onClick={() => { setQ(ex); run(ex); }}>{ex}</button>
          ))}
        </div>

        {res?.error ? (
          <div className="banner" style={{ marginTop: 18 }}>{t.searchDisabled}</div>
        ) : (
          <>
            <h2 className="section">
              {loading ? t.loading : res ? t.searchTotal(res.total) : ""}
            </h2>
            <div className="panel">
              {res && res.hits.length === 0 && !loading && (
                <div className="feed-row muted">{t.searchNone}</div>
              )}
              {res?.hits.map((h, i) => {
                const ev = h.event;
                const team = (ev.team || "?").toLowerCase();
                const tag = team === "red" ? "red" : team === "green" ? "green" : "blue";
                return (
                  <div className="feed-row" key={ev.id ?? i}>
                    <span className={`tag ${tag}`}>{(ev.team || "?").toUpperCase()}</span>
                    <span className="tech" style={{ minWidth: 74 }}>{ev.technique || ev.kind || ""}</span>
                    <span className="detail">{ev.detail || ev.variant || ""}</span>
                    <span className={`out ${ev.outcome || ""}`}>{ev.outcome || ""}</span>
                    <span className="muted" style={{ fontSize: 11, marginLeft: 8, flex: "0 0 auto" }}>
                      {ev.ts ? new Date(ev.ts).toLocaleTimeString() : ""}
                    </span>
                  </div>
                );
              })}
            </div>
          </>
        )}

        <div className="foot-note">{t.footNote}</div>
      </div>
    </>
  );
}
