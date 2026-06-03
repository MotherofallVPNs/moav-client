import { useEffect, useMemo, useState } from "react";
import { theme } from "../theme";
import { API_BASE, WS_BASE } from "../apiBase";


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
  proxy: { bg: theme.blueDim, fg: theme.blue },
  direct: { bg: theme.greenDim, fg: theme.green },
  block: { bg: theme.redDim, fg: theme.red },
};

const td: React.CSSProperties = { padding: "0.5rem 0.6rem", fontSize: "0.78rem", verticalAlign: "middle" };
const th: React.CSSProperties = {
  ...td,
  textAlign: "left",
  fontWeight: 500,
  color: theme.textDim,
  background: theme.surface2,
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  letterSpacing: "0.04em",
  textTransform: "uppercase" as const,
  borderBottom: `1px solid ${theme.border}`,
};

const inputStyle: React.CSSProperties = {
  padding: "0.35rem 0.55rem",
  borderRadius: 4,
  fontSize: "0.82rem",
  fontFamily: theme.mono,
};

interface Props {
  refreshTick?: number;
}

export default function Plugins({ refreshTick }: Props) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [draft, setDraft] = useState<Rule | null>(null);

  useEffect(() => {
    refresh();
  }, [refreshTick]);

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
    flash(`Added ${fresh.length} rule${fresh.length > 1 ? "s" : ""} from "${tpl.title}". Toggle on to activate.`, true);
  };

  // Edit-in-place: index of the rule currently being edited, plus a working
  // copy. Save replaces; cancel reverts.
  const [editIdx, setEditIdx] = useState<number | null>(null);
  const [editDraft, setEditDraft] = useState<Rule | null>(null);

  const startEdit = (i: number) => {
    setEditIdx(i);
    setEditDraft({ ...rules[i] });
  };
  const updateEditDraft = (patch: Partial<Rule>) =>
    setEditDraft((d) => (d ? { ...d, ...patch } : d));
  const updateEditMatch = (patch: Partial<Rule["match"]>) =>
    setEditDraft((d) => (d ? { ...d, match: { ...d.match, ...patch } } : d));
  const commitEdit = () => {
    if (editIdx === null || !editDraft) return;
    if (!editDraft.match.value) {
      flash("Value required.", false);
      return;
    }
    const next = rules.map((r, i) => (i === editIdx ? editDraft : r));
    persist(next);
    setEditIdx(null);
    setEditDraft(null);
  };
  const cancelEdit = () => {
    setEditIdx(null);
    setEditDraft(null);
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
          <h3 style={{ margin: 0, fontFamily: theme.mono, fontSize: "0.85rem", color: theme.text }}>routing rules</h3>
          <div style={{ fontSize: "0.72rem", color: theme.textDim, marginTop: 2, fontFamily: theme.mono }}>
            first-match-wins · {ruleCount} · changes apply live
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button onClick={() => setPickerOpen((p) => !p)} style={ghostBtn(theme.blue)}>
            + from template…
          </button>
          <button onClick={() => setDraft(blankDraft())} style={solidBtn(theme.blue)}>
            + new rule
          </button>
        </div>
      </div>

      {pickerOpen && (
        <div
          style={{
            marginBottom: "1rem",
            border: `1px solid ${theme.border}`,
            borderRadius: 6,
            padding: "0.75rem",
            background: theme.surface2,
          }}
        >
          <div style={{ fontSize: "0.78rem", color: theme.textDim, marginBottom: "0.5rem", fontFamily: theme.mono }}>
            curated templates — all rules land disabled so you can review before enabling.
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: "0.6rem" }}>
            {templates.map((tpl) => (
              <div
                key={tpl.key}
                style={{
                  border: `1px solid ${theme.border}`,
                  borderRadius: 6,
                  padding: "0.7rem",
                  background: theme.surface,
                }}
              >
                <div style={{ fontFamily: theme.mono, fontSize: "0.82rem", color: theme.green }}>{tpl.title}</div>
                <div style={{ fontSize: "0.72rem", color: theme.textDim, marginTop: 4, lineHeight: 1.45 }}>{tpl.help}</div>
                <div style={{ fontSize: "0.7rem", color: theme.textDim, marginTop: 6, fontFamily: theme.mono }}>
                  {tpl.rules.length} rule{tpl.rules.length > 1 ? "s" : ""}
                </div>
                <button onClick={() => fromTemplate(tpl)} style={{ ...solidBtn(theme.green), marginTop: "0.55rem" }}>
                  add (disabled)
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            {["#", "On", "Match", "Value", "Action", "Note", ""].map((h) => (
              <th key={h} style={th}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rules.length === 0 ? (
            <tr>
              <td colSpan={7} style={{ ...td, color: theme.textDim }}>
                No rules configured. Add one above or pick a template.
              </td>
            </tr>
          ) : (
            rules.map((r, i) =>
              editIdx === i && editDraft ? (
                // Inline edit row.
                <tr key={(r.id ?? "") + i + "-edit"} style={{ borderTop: `1px solid ${theme.blue}`, background: theme.blueDim }}>
                  <td style={{ ...td, color: theme.textDim }}>{i + 1}</td>
                  <td style={td}>
                    <input
                      type="checkbox"
                      checked={editDraft.enabled}
                      onChange={(e) => updateEditDraft({ enabled: e.target.checked })}
                    />
                  </td>
                  <td style={td}>
                    <select
                      value={editDraft.match.type}
                      onChange={(e) => updateEditMatch({ type: e.target.value as MatchType })}
                      style={{ ...inputStyle, width: "100%" }}
                    >
                      {MATCH_TYPES.map((t) => (
                        <option key={t} value={t}>
                          {t}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td style={td}>
                    <input
                      type="text"
                      value={editDraft.match.value}
                      onChange={(e) => updateEditMatch({ value: e.target.value })}
                      style={{ ...inputStyle, width: "100%" }}
                    />
                  </td>
                  <td style={td}>
                    <select
                      value={editDraft.action}
                      onChange={(e) => updateEditDraft({ action: e.target.value as Action })}
                      style={{ ...inputStyle, width: "100%" }}
                    >
                      {ACTIONS.map((a) => (
                        <option key={a} value={a}>
                          {a}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td style={td}>
                    <input
                      type="text"
                      value={editDraft.note ?? ""}
                      onChange={(e) => updateEditDraft({ note: e.target.value })}
                      placeholder="(optional)"
                      style={{ ...inputStyle, width: "100%" }}
                    />
                  </td>
                  <td style={{ ...td, whiteSpace: "nowrap" }}>
                    <button onClick={commitEdit} style={{ ...iconBtn(false), color: theme.green }} title="Save">✓</button>
                    <button onClick={cancelEdit} style={iconBtn(false)} title="Cancel">×</button>
                  </td>
                </tr>
              ) : (
                <tr key={(r.id ?? "") + i} style={{ borderTop: `1px solid ${theme.border}` }}>
                  <td style={{ ...td, color: theme.textDim }}>{i + 1}</td>
                  <td style={td}>
                    <input type="checkbox" checked={r.enabled} onChange={() => toggle(i)} />
                  </td>
                  <td style={{ ...td, fontFamily: theme.mono, color: theme.text }}>{r.match.type}</td>
                  <td style={{ ...td, fontFamily: theme.mono, color: theme.blue, maxWidth: 240, wordBreak: "break-all" }}>
                    {r.match.value}
                  </td>
                  <td style={td}>
                    <span
                      style={{
                        display: "inline-block",
                        padding: "0.15rem 0.55rem",
                        background: ACTION_STYLE[r.action].bg,
                        color: ACTION_STYLE[r.action].fg,
                        borderRadius: 12,
                        fontSize: "0.65rem",
                        fontWeight: 600,
                        fontFamily: theme.mono,
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        border: `1px solid ${ACTION_STYLE[r.action].fg}44`,
                      }}
                    >
                      {r.action}
                    </span>
                  </td>
                  <td style={{ ...td, color: theme.textDim, maxWidth: 220 }}>{r.note || "—"}</td>
                  <td style={{ ...td, whiteSpace: "nowrap" }}>
                    <button onClick={() => startEdit(i)} style={iconBtn(false)} title="Edit">✎</button>
                    <button onClick={() => move(i, -1)} disabled={i === 0} style={iconBtn(i === 0)}>↑</button>
                    <button onClick={() => move(i, 1)} disabled={i === rules.length - 1} style={iconBtn(i === rules.length - 1)}>↓</button>
                    <button onClick={() => remove(i)} style={{ ...iconBtn(false), color: theme.red }}>×</button>
                  </td>
                </tr>
              )
            )
          )}
        </tbody>
      </table>

      {draft && (
        <div
          style={{
            marginTop: "1rem",
            border: `1px solid ${theme.blue}`,
            borderRadius: 6,
            padding: "0.75rem",
            background: theme.blueDim,
          }}
        >
          <div style={{ fontSize: "0.85rem", fontWeight: 600, marginBottom: "0.5rem", color: theme.text, fontFamily: theme.mono }}>
            new rule
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "0.5rem 0.75rem", alignItems: "center" }}>
            <label style={lbl}>match type</label>
            <select
              value={draft.match.type}
              onChange={(e) => updateDraftMatch({ type: e.target.value as MatchType })}
              style={inputStyle}
            >
              {MATCH_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
            <label style={lbl}>value</label>
            <input
              type="text"
              value={draft.match.value}
              onChange={(e) => updateDraftMatch({ value: e.target.value })}
              placeholder={placeholderFor(draft.match.type)}
              style={inputStyle}
            />
            <label style={lbl}>action</label>
            <select value={draft.action} onChange={(e) => updateDraft({ action: e.target.value as Action })} style={inputStyle}>
              {ACTIONS.map((a) => (
                <option key={a} value={a}>
                  {a}
                </option>
              ))}
            </select>
            <label style={lbl}>note</label>
            <input
              type="text"
              value={draft.note ?? ""}
              onChange={(e) => updateDraft({ note: e.target.value })}
              placeholder="(optional)"
              style={inputStyle}
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
              style={solidBtn(theme.blue)}
            >
              add rule
            </button>
            <button onClick={() => setDraft(null)} style={ghostBtn(theme.textDim)}>
              cancel
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
            background: toast.ok ? theme.green : theme.red,
            color: theme.bg,
            borderRadius: 4,
            fontSize: "0.78rem",
            fontFamily: theme.mono,
            fontWeight: 600,
            maxWidth: 360,
          }}
        >
          {toast.msg}
        </div>
      )}
    </div>
  );
}

const lbl: React.CSSProperties = { fontSize: "0.72rem", color: theme.textDim, fontFamily: theme.mono };

const solidBtn = (color: string): React.CSSProperties => ({
  padding: "0.35rem 0.8rem",
  background: color,
  color: theme.bg,
  border: "none",
  borderRadius: 4,
  cursor: "pointer",
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
});

const ghostBtn = (color: string): React.CSSProperties => ({
  padding: "0.35rem 0.8rem",
  background: "transparent",
  color: color,
  border: `1px solid ${color}`,
  borderRadius: 4,
  cursor: "pointer",
  fontFamily: theme.mono,
  fontSize: "0.7rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
});

const iconBtn = (disabled: boolean): React.CSSProperties => ({
  padding: "0 0.4rem",
  marginRight: 4,
  background: "transparent",
  border: `1px solid ${theme.border}`,
  borderRadius: 4,
  cursor: disabled ? "not-allowed" : "pointer",
  fontSize: "0.85rem",
  color: disabled ? theme.border : theme.textDim,
  fontFamily: theme.mono,
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
