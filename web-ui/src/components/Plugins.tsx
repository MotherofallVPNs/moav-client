import { useEffect, useMemo, useState } from "react";

const API_BASE = (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8088";

type Action = "proxy" | "direct" | "block";
type MatchType =
  | "domain"
  | "domain_suffix"
  | "domain_keyword"
  | "ip_cidr"
  | "geoip"
  | "port"
  | "protocol";

interface Rule {
  id?: string;
  enabled: boolean;
  action: Action;
  note?: string;
  match: { type: MatchType; value: string };
}

interface Template {
  key: string;
  title: string;
  help: string;
  rules: Rule[];
}

const MATCH_TYPES: MatchType[] = [
  "domain",
  "domain_suffix",
  "domain_keyword",
  "ip_cidr",
  "geoip",
  "port",
  "protocol",
];
const ACTIONS: Action[] = ["proxy", "direct", "block"];

const ACTION_STYLE: Record<Action, { bg: string; fg: string }> = {
  proxy: { bg: "#dbeafe", fg: "#1d4ed8" },
  direct: { bg: "#dcfce7", fg: "#15803d" },
  block: { bg: "#fee2e2", fg: "#b91c1c" },
};

const td: React.CSSProperties = { padding: "0.5rem 0.6rem", fontSize: "0.85rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: "#475569",
  background: "#f8fafc",
};

export default function Plugins() {
  const [rules, setRules] = useState<Rule[]>([]);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [draft, setDraft] = useState<Rule | null>(null);

  useEffect(() => {
    refresh();
  }, []);

  const refresh = async () => {
    try {
      const r = await fetch(`${API_BASE}/api/plugins`);
      const data = await r.json();
      setRules((data.rules ?? []) as Rule[]);
      setTemplates((data.templates ?? []) as Template[]);
    } catch {
      flash("Could not load plugins", false);
    }
  };

  const flash = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 2500);
  };

  const persist = async (next: Rule[]) => {
    const prev = rules;
    setRules(next);
    try {
      const r = await fetch(`${API_BASE}/api/plugins`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ rules: next }),
      });
      if (!r.ok) throw new Error(`${r.status}`);
      const data = await r.json();
      setRules((data.rules ?? next) as Rule[]);
      flash("Applied.", true);
    } catch {
      setRules(prev);
      flash("Apply failed.", false);
    }
  };

  const toggle = (idx: number) => {
    const next = rules.map((r, i) => (i === idx ? { ...r, enabled: !r.enabled } : r));
    persist(next);
  };

  const remove = (idx: number) => {
    const next = rules.filter((_, i) => i !== idx);
    persist(next);
  };

  const move = (idx: number, delta: number) => {
    const next = rules.slice();
    const tgt = idx + delta;
    if (tgt < 0 || tgt >= next.length) return;
    [next[idx], next[tgt]] = [next[tgt], next[idx]];
    persist(next);
  };

  const addRule = (rule: Rule) => persist([...rules, rule]);

  const fromTemplate = (tpl: Template) => {
    const fresh = tpl.rules.map((r) => ({ ...r, enabled: false, id: undefined }));
    persist([...rules, ...fresh]);
    setPickerOpen(false);
    flash(`Added ${fresh.length} rule${fresh.length > 1 ? "s" : ""} from "${tpl.title}". Toggle enable to activate.`, true);
  };

  const updateDraft = (patch: Partial<Rule>) => setDraft((d) => (d ? { ...d, ...patch } : d));
  const updateDraftMatch = (patch: Partial<Rule["match"]>) =>
    setDraft((d) => (d ? { ...d, match: { ...d.match, ...patch } } : d));

  const blankDraft = (): Rule => ({
    enabled: false,
    action: "block",
    note: "",
    match: { type: "domain_suffix", value: "" },
  });

  const ruleCount = useMemo(() => {
    const on = rules.filter((r) => r.enabled).length;
    return `${on} active / ${rules.length} total`;
  }, [rules]);

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "0.75rem" }}>
        <div>
          <h3 style={{ margin: 0, fontSize: "0.95rem", color: "#0f172a" }}>Routing rules</h3>
          <div style={{ fontSize: "0.8rem", color: "#64748b", marginTop: 2 }}>
            First-match-wins. {ruleCount}. Changes apply live — no restart.
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button
            onClick={() => setPickerOpen((p) => !p)}
            style={{
              padding: "0.4rem 0.9rem",
              background: "#fff",
              color: "#1d4ed8",
              border: "1px solid #1d4ed8",
              borderRadius: 6,
              cursor: "pointer",
              fontWeight: 500,
              fontSize: "0.85rem",
            }}
          >
            + From template…
          </button>
          <button
            onClick={() => setDraft(blankDraft())}
            style={{
              padding: "0.4rem 0.9rem",
              background: "#1d4ed8",
              color: "#fff",
              border: "1px solid #1d4ed8",
              borderRadius: 6,
              cursor: "pointer",
              fontWeight: 500,
              fontSize: "0.85rem",
            }}
          >
            + New rule
          </button>
        </div>
      </div>

      {pickerOpen && (
        <div style={{ marginBottom: "1rem", border: "1px solid #e2e8f0", borderRadius: 8, padding: "0.75rem" }}>
          <div style={{ fontSize: "0.85rem", color: "#475569", marginBottom: "0.5rem" }}>
            Pick a curated template — all rules land disabled so you can review before enabling.
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: "0.6rem" }}>
            {templates.map((tpl) => (
              <div
                key={tpl.key}
                style={{
                  border: "1px solid #e2e8f0",
                  borderRadius: 8,
                  padding: "0.75rem",
                  background: "#fff",
                }}
              >
                <div style={{ fontWeight: 600, fontSize: "0.9rem", color: "#0f172a" }}>{tpl.title}</div>
                <div style={{ fontSize: "0.78rem", color: "#64748b", marginTop: 4, lineHeight: 1.4 }}>{tpl.help}</div>
                <div style={{ fontSize: "0.72rem", color: "#94a3b8", marginTop: 6 }}>
                  {tpl.rules.length} rule{tpl.rules.length > 1 ? "s" : ""}
                </div>
                <button
                  onClick={() => fromTemplate(tpl)}
                  style={{
                    marginTop: "0.5rem",
                    padding: "0.3rem 0.7rem",
                    background: "#22c55e",
                    color: "#fff",
                    border: "none",
                    borderRadius: 5,
                    cursor: "pointer",
                    fontSize: "0.78rem",
                  }}
                >
                  Add to ruleset (disabled)
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["#", "Enabled", "Match", "Value", "Action", "Note", ""].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rules.length === 0 ? (
            <tr>
              <td colSpan={7} style={{ ...td, color: "#94a3b8" }}>
                No rules configured. Add one above or pick a template.
              </td>
            </tr>
          ) : (
            rules.map((r, i) => (
              <tr key={(r.id ?? "") + i} style={{ borderTop: "1px solid #e2e8f0" }}>
                <td style={{ ...td, color: "#94a3b8" }}>{i + 1}</td>
                <td style={td}>
                  <label style={{ cursor: "pointer" }}>
                    <input type="checkbox" checked={r.enabled} onChange={() => toggle(i)} />
                  </label>
                </td>
                <td style={{ ...td, fontFamily: "monospace", fontSize: "0.78rem" }}>{r.match.type}</td>
                <td style={{ ...td, fontFamily: "monospace", fontSize: "0.78rem", maxWidth: 240, wordBreak: "break-all" }}>
                  {r.match.value}
                </td>
                <td style={td}>
                  <span
                    style={{
                      display: "inline-block",
                      padding: "0.15rem 0.55rem",
                      background: ACTION_STYLE[r.action].bg,
                      color: ACTION_STYLE[r.action].fg,
                      borderRadius: 10,
                      fontSize: "0.72rem",
                      fontWeight: 600,
                      textTransform: "uppercase",
                    }}
                  >
                    {r.action}
                  </span>
                </td>
                <td style={{ ...td, color: "#64748b", fontSize: "0.78rem", maxWidth: 220 }}>
                  {r.note || "—"}
                </td>
                <td style={{ ...td, whiteSpace: "nowrap" }}>
                  <button onClick={() => move(i, -1)} disabled={i === 0} style={iconBtn(i === 0)}>↑</button>
                  <button onClick={() => move(i, 1)} disabled={i === rules.length - 1} style={iconBtn(i === rules.length - 1)}>↓</button>
                  <button onClick={() => remove(i)} style={{ ...iconBtn(false), color: "#b91c1c" }}>×</button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      {draft && (
        <div style={{ marginTop: "1rem", border: "1px solid #1d4ed8", borderRadius: 8, padding: "0.75rem", background: "#eff6ff" }}>
          <div style={{ fontSize: "0.9rem", fontWeight: 600, marginBottom: "0.5rem", color: "#0f172a" }}>New rule</div>
          <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "0.5rem 0.75rem", alignItems: "center" }}>
            <label style={lbl}>Match type</label>
            <select
              value={draft.match.type}
              onChange={(e) => updateDraftMatch({ type: e.target.value as MatchType })}
              style={input}
            >
              {MATCH_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
            <label style={lbl}>Value</label>
            <input
              type="text"
              value={draft.match.value}
              onChange={(e) => updateDraftMatch({ value: e.target.value })}
              placeholder={placeholderFor(draft.match.type)}
              style={input}
            />
            <label style={lbl}>Action</label>
            <select value={draft.action} onChange={(e) => updateDraft({ action: e.target.value as Action })} style={input}>
              {ACTIONS.map((a) => (
                <option key={a} value={a}>
                  {a}
                </option>
              ))}
            </select>
            <label style={lbl}>Note</label>
            <input
              type="text"
              value={draft.note ?? ""}
              onChange={(e) => updateDraft({ note: e.target.value })}
              placeholder="(optional)"
              style={input}
            />
          </div>
          <div style={{ marginTop: "0.75rem", display: "flex", gap: "0.5rem" }}>
            <button
              onClick={() => {
                if (!draft.match.value) {
                  flash("Value required.", false);
                  return;
                }
                addRule({ ...draft, enabled: true });
                setDraft(null);
              }}
              style={{
                padding: "0.4rem 0.9rem",
                background: "#1d4ed8",
                color: "#fff",
                border: "none",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: "0.85rem",
                fontWeight: 500,
              }}
            >
              Add rule
            </button>
            <button
              onClick={() => setDraft(null)}
              style={{
                padding: "0.4rem 0.9rem",
                background: "#fff",
                color: "#64748b",
                border: "1px solid #cbd5e1",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: "0.85rem",
              }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {toast && (
        <div
          style={{
            position: "fixed",
            bottom: 24,
            right: 24,
            padding: "0.6rem 1rem",
            background: toast.ok ? "#16a34a" : "#dc2626",
            color: "#fff",
            borderRadius: 6,
            fontSize: "0.85rem",
            boxShadow: "0 4px 14px rgba(0,0,0,0.15)",
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

const lbl: React.CSSProperties = { fontSize: "0.8rem", color: "#475569", fontWeight: 500 };
const input: React.CSSProperties = {
  padding: "0.35rem 0.55rem",
  border: "1px solid #cbd5e1",
  borderRadius: 5,
  fontSize: "0.85rem",
  fontFamily: "monospace",
};

const iconBtn = (disabled: boolean): React.CSSProperties => ({
  padding: "0 0.4rem",
  marginRight: 4,
  background: "transparent",
  border: "1px solid #e2e8f0",
  borderRadius: 4,
  cursor: disabled ? "not-allowed" : "pointer",
  fontSize: "0.9rem",
  color: disabled ? "#cbd5e1" : "#475569",
});

function placeholderFor(t: MatchType): string {
  switch (t) {
    case "domain":
      return "example.com";
    case "domain_suffix":
      return "example.com  (matches *.example.com too)";
    case "domain_keyword":
      return "torrent";
    case "ip_cidr":
      return "10.0.0.0/8";
    case "geoip":
      return "ir";
    case "port":
      return "443  or  1000-2000";
    case "protocol":
      return "tcp  or  udp";
  }
}
