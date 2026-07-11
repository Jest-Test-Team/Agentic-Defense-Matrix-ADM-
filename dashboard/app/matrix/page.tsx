"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { translations, getLang, setLang, type Lang } from "@/lib/i18n";

interface Variant {
  id: string;
  technique: string;
  name: string;
  tag: string;
  mutation: string;
  lang: string;
  severity: number;
  target: string;
  preview: string;
}

const PAGE = 100;

export default function MatrixPage() {
  const [lang, setLangState] = useState<Lang>("en");
  const t = translations[lang];
  const [all, setAll] = useState<Variant[] | null>(null);
  const [q, setQ] = useState("");
  const [family, setFamily] = useState("");
  const [page, setPage] = useState(0);

  useEffect(() => {
    setLangState(getLang());
    // Static asset under the Pages basePath — fetch relative to this page.
    fetch("../corpus.json")
      .then((r) => r.json())
      .then((d) => setAll(d.variants ?? []))
      .catch(() => setAll([]));
  }, []);

  const switchLang = (l: Lang) => {
    setLang(l);
    setLangState(l);
  };

  const families = useMemo(() => {
    if (!all) return [];
    const seen = new Map<string, string>();
    for (const v of all) if (!seen.has(v.technique)) seen.set(v.technique, v.name);
    return [...seen.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [all]);

  const filtered = useMemo(() => {
    if (!all) return [];
    const needle = q.trim().toLowerCase();
    return all.filter((v) => {
      if (family && v.technique !== family) return false;
      if (!needle) return true;
      return (
        v.id.toLowerCase().includes(needle) ||
        v.technique.toLowerCase().includes(needle) ||
        v.name.toLowerCase().includes(needle) ||
        v.tag.toLowerCase().includes(needle) ||
        v.mutation.toLowerCase().includes(needle) ||
        v.preview.toLowerCase().includes(needle)
      );
    });
  }, [all, q, family]);

  // Reset to first page whenever the filter changes.
  useEffect(() => setPage(0), [q, family]);

  const total = filtered.length;
  const start = page * PAGE;
  const rows = filtered.slice(start, start + PAGE);
  const maxPage = Math.max(0, Math.ceil(total / PAGE) - 1);

  return (
    <>
      <header className="top">
        <div className="top-inner">
          <h1 className="brand">🎯 {t.matrixFullTitle}</h1>
          <div className="langtoggle">
            <button className={lang === "en" ? "on" : ""} onClick={() => switchLang("en")}>EN</button>
            <button className={lang === "zh-Hant" ? "on" : ""} onClick={() => switchLang("zh-Hant")}>繁中</button>
          </div>
          <Link className="navlink" href="/search">{t.searchNav}</Link>
          <a className="navlink ghost" href="https://github.com/Jest-Test-Team/Agentic-Defense-Matrix-ADM" target="_blank" rel="noopener noreferrer">⭐ {t.githubLink}</a>
          <div className="conn">
            <Link href="/">{t.backToConsole}</Link>
          </div>
        </div>
      </header>

      <div className="wrap">
        <p className="muted" style={{ margin: "18px 2px 12px", fontSize: 13, lineHeight: 1.65 }}>{t.matrixIntro}</p>

        <div className="searchbar">
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t.matrixSearchPlaceholder} aria-label="filter" />
          <select className="famselect" value={family} onChange={(e) => setFamily(e.target.value)}>
            <option value="">{t.matrixAllFamilies}</option>
            {families.map(([id, name]) => (
              <option key={id} value={id}>{id} · {name}</option>
            ))}
          </select>
        </div>

        <div className="matrix-toolbar">
          <span className="muted">{all == null ? t.matrixLoading : t.matrixShowing(total === 0 ? 0 : start + 1, Math.min(start + PAGE, total), total)}</span>
          <span className="pager">
            <button disabled={page <= 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>{t.matrixPrev}</button>
            <button disabled={page >= maxPage} onClick={() => setPage((p) => Math.min(maxPage, p + 1))}>{t.matrixNext}</button>
          </span>
        </div>

        <div className="matrix-panel">
          <table className="attack-matrix wide">
            <thead>
              <tr>
                <th>{t.matrixId}</th>
                <th>{t.matrixTechnique}</th>
                <th>{t.matrixAttack}</th>
                <th>{t.matrixColMutation}</th>
                <th>{t.matrixColLang}</th>
                <th>{t.matrixColSeverity}</th>
                <th>{t.matrixColTarget}</th>
                <th>{t.matrixColPreview}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((v) => (
                <tr key={v.id}>
                  <td className="matrix-id">{v.id}</td>
                  <td>{v.technique}</td>
                  <td>{v.name} <span className="muted" style={{ fontSize: 11 }}>{v.tag}</span></td>
                  <td>{v.mutation}</td>
                  <td>{v.lang}</td>
                  <td><span className={`sevdot s${v.severity}`}>{v.severity}</span></td>
                  <td>{v.target}</td>
                  <td className="matrix-prev">{v.preview}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="matrix-toolbar" style={{ marginTop: 12 }}>
          <span />
          <span className="pager">
            <button disabled={page <= 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>{t.matrixPrev}</button>
            <button disabled={page >= maxPage} onClick={() => setPage((p) => Math.min(maxPage, p + 1))}>{t.matrixNext}</button>
          </span>
        </div>
      </div>
    </>
  );
}
