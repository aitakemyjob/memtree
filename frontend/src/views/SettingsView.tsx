import { useEffect, useState, type ReactNode } from 'react';
import { Database, Info, Palette, Plug, Shield, SlidersHorizontal, Webhook, type LucideIcon } from 'lucide-react';
import { hoursLabel } from '../lib/format';
import { needsRestart, settingsChanged, validateSettings } from '../lib/settings';
import { useStore } from '../lib/store';
import { themes } from '../lib/themes';
import type { Settings } from '../lib/types';
import { ConnectPanel } from '../components/ConnectPanel';
import { HooksPanel } from '../components/HooksPanel';
import { Field, Switch } from '../components/ui';

type Section = 'appearance' | 'agents' | 'memory' | 'hooks' | 'connect' | 'about';
const SECTIONS: { id: Section; label: string; icon: LucideIcon }[] = [
  { id: 'appearance', label: 'Appearance', icon: Palette },
  { id: 'agents', label: 'Agent access', icon: Shield },
  { id: 'memory', label: 'Memory tuning', icon: SlidersHorizontal },
  { id: 'hooks', label: 'Hooks', icon: Webhook },
  { id: 'connect', label: 'Connect (MCP)', icon: Plug },
  { id: 'about', label: 'About', icon: Info },
];

/** Switch with a title and an explanation, used for the agent-access toggles. */
function ToggleRow({ title, on, onChange, children }: { title: string; on: boolean; onChange: (v: boolean) => void; children: ReactNode }) {
  return (
    <div className="toggle-row">
      <Switch on={on} onChange={onChange} label={title} />
      <div className="grow">
        <b>{title}</b>
        <div className="muted">{children}</div>
      </div>
    </div>
  );
}

/** Numeric input that keeps its own text so partial typing is not clobbered. */
function NumberField({ label, value, onChange, min, max, step = 1, suffix }: { label: string; value: number; onChange: (n: number) => void; min?: number; max?: number; step?: number; suffix?: string }) {
  return (
    <div className="row">
      <input className="input" style={{ width: 110 }} type="number" aria-label={label} value={Number.isFinite(value) ? value : ''} min={min} max={max} step={step} onChange={(e) => onChange(e.target.value === '' ? NaN : Number(e.target.value))} />
      {suffix && <span className="faint">{suffix}</span>}
    </div>
  );
}

/**
 * Settings: appearance, agent access, memory tuning, hooks, MCP snippets
 * and about. Edits are held in a draft and saved as the WHOLE settings
 * object (the backend replaces it), so nothing else is clobbered.
 */
export function SettingsView() {
  const s = useStore();
  const tab = s.tabs.find((t) => t.id === s.active);
  const requested = (tab?.params.section as Section | undefined) ?? 'appearance';
  const [section, setSection] = useState<Section>(requested);
  useEffect(() => setSection(requested), [requested, tab?.params]);

  const saved = s.app?.settings;
  const [draft, setDraft] = useState<Settings | undefined>(saved);
  const [savedKey, setSavedKey] = useState(JSON.stringify(saved));
  // Adopt backend changes (another window, a config event) unless the user is mid-edit.
  useEffect(() => {
    if (!saved) return;
    const key = JSON.stringify(saved);
    if (key === savedKey) return;
    if (!draft || !settingsChanged(JSON.parse(savedKey) as Settings, draft)) setDraft(saved);
    setSavedKey(key);
  }, [saved, savedKey, draft]);

  if (!saved || !draft) return null;
  const dirty = settingsChanged(saved, draft);
  const problems = validateSettings(draft);
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => setDraft({ ...draft, [k]: v });
  const setTier = <K extends keyof Settings['tiers']>(k: K, v: number) => setDraft({ ...draft, tiers: { ...draft.tiers, [k]: v } });
  const go = (id: Section) => {
    setSection(id);
    if (tab) s.updateTab(tab.id, { section: id });
  };

  return (
    <div className="view">
      <div className="split">
        <nav className="side settings-nav" style={{ width: 210 }} aria-label="Settings sections">
          {SECTIONS.map((x) => (
            <button key={x.id} className={`list-item ${section === x.id ? 'on' : ''}`} style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left', alignItems: 'center' }} onClick={() => go(x.id)} aria-current={section === x.id ? 'page' : undefined}>
              <x.icon size={15} />
              <span className="title">{x.label}</span>
            </button>
          ))}
        </nav>
        <div className="main">
          <div className="scroll grow">
            <div className="page">
              {section === 'appearance' && (
                <>
                  <h1>Appearance</h1>
                  <div className="theme-swatches" role="radiogroup" aria-label="Theme">
                    {themes.map((t) => (
                      <button
                        key={t.id}
                        role="radio"
                        aria-checked={t.id === s.theme.id}
                        className={`swatch ${t.id === s.theme.id ? 'on' : ''}`}
                        style={{ padding: 0, textAlign: 'left', background: 'var(--bg-1)' }}
                        onClick={() => {
                          setDraft((d) => d && { ...d, theme: t.id });
                          void s.setTheme(t.id);
                        }}
                      >
                        <div className="preview" style={{ background: t.bg }}>
                          <i style={{ background: t.bg2, border: `1px solid ${t.borderStrong}` }} />
                          <i style={{ background: t.accent }} />
                          <i style={{ background: t.text2 }} />
                          <i style={{ background: t.heat[3] }} />
                        </div>
                        <div className="name">{t.name} <span className="faint">{t.mode}</span></div>
                      </button>
                    ))}
                  </div>
                  <Field label="Font size" hint="Applies to the whole interface.">
                    <NumberField label="Font size" value={draft.fontSize} min={10} max={22} onChange={(n) => set('fontSize', n)} suffix="px" />
                  </Field>
                </>
              )}

              {section === 'agents' && (
                <>
                  <h1>Agent access</h1>
                  <ToggleRow title="Agent writes" on={draft.agentWrites} onChange={(v) => set('agentWrites', v)}>
                    {draft.agentWrites
                      ? 'Agents can save, update, supersede and archive memories over MCP. Turn it off for a read-only recall setup.'
                      : 'Off: agents can recall and read memories but cannot change anything. Hooks still record sessions if capture is on.'}
                  </ToggleRow>
                  <ToggleRow title="Capture hooks" on={draft.captureHooks} onChange={(v) => set('captureHooks', v)}>
                    Lets harness hooks record conversation turns into capture sessions, which agents later summarise. Off stops new captures; existing ones stay.
                  </ToggleRow>
                  <Field
                    label={`Surface threshold: ${draft.surfaceThreshold.toFixed(2)}`}
                    hint="Hooks inject memories on each prompt only when they score at least this much. Higher is quieter: nothing is injected unless something clearly relates to the prompt. Lower surfaces more, with more noise."
                  >
                    <div className="row">
                      <span className="faint">quiet</span>
                      <input type="range" aria-label="Surface threshold" className="range" min={0} max={1} step={0.05} value={Number.isFinite(draft.surfaceThreshold) ? draft.surfaceThreshold : 0} onChange={(e) => set('surfaceThreshold', Number(e.target.value))} />
                      <span className="faint">chatty</span>
                    </div>
                  </Field>
                  <Field label="Max surfaced per prompt" hint="Upper bound on memories a hook injects for one prompt.">
                    <NumberField label="Max surfaced per prompt" value={draft.maxSurface} min={1} max={10} onChange={(n) => set('maxSurface', n)} />
                  </Field>
                </>
              )}

              {section === 'memory' && (
                <>
                  <h1>Memory tuning</h1>
                  <Field label="Link half-life" hint="Lattice links fade with disuse; links that have faded away are dropped. Confirming or recalling a memory refreshes them.">
                    <NumberField label="Link half-life" value={draft.linkHalfLifeDays} min={1} onChange={(n) => set('linkHalfLifeDays', n)} suffix="days" />
                  </Field>
                  <Field label="Maintenance interval" hint="How often background maintenance (link decay) runs.">
                    <NumberField label="Maintenance interval" value={draft.maintainHours} min={1} onChange={(n) => set('maintainHours', n)} suffix="hours" />
                  </Field>
                  <h2 className="sec">Retrieval traces</h2>
                  <ToggleRow title="Trace retrievals" on={draft.traceRetrievals} onChange={(v) => set('traceRetrievals', v)}>
                    Record why each memory was recalled or injected, and whether the agent then used it, for the Traces view, retrieval history and Timeline heat. Stays on this machine.
                  </ToggleRow>
                  <Field label="Keep traces for" hint="Older traces are pruned during maintenance. 0 keeps them forever.">
                    <NumberField label="Trace retention" value={draft.traceRetentionDays} min={0} onChange={(n) => set('traceRetentionDays', n)} suffix="days" />
                  </Field>
                  <h2 className="sec">Consolidation tiers</h2>
                  <Field label="Episodic age gate" hint={`Episodic memories can be distilled after ${hoursLabel(draft.tiers.ageHoursToSemantic)}.`}>
                    <NumberField label="Episodic age gate" value={draft.tiers.ageHoursToSemantic} min={1} onChange={(n) => setTier('ageHoursToSemantic', n)} suffix="hours" />
                  </Field>
                  <Field label="Semantic age gate" hint={`Semantic gists can be distilled again after ${hoursLabel(draft.tiers.ageHoursToCompact)}.`}>
                    <NumberField label="Semantic age gate" value={draft.tiers.ageHoursToCompact} min={1} onChange={(n) => setTier('ageHoursToCompact', n)} suffix="hours" />
                  </Field>
                  <Field label="Activation ceiling" hint="A memory still warmer than this is never compressed, however old.">
                    <NumberField label="Activation ceiling" value={draft.tiers.activationCeilingToCompress} min={0.15} max={1} step={0.05} onChange={(n) => setTier('activationCeilingToCompress', n)} />
                  </Field>
                  <Field label="Minimum cluster size" hint="Related memories needed before a group is offered for distillation.">
                    <NumberField label="Minimum cluster size" value={draft.tiers.minClusterSize} min={2} onChange={(n) => setTier('minClusterSize', n)} />
                  </Field>
                </>
              )}

              {section === 'hooks' && (
                <>
                  <h1>Hooks</h1>
                  <HooksPanel />
                </>
              )}

              {section === 'connect' && (
                <>
                  <h1>Connect (MCP)</h1>
                  <ConnectPanel />
                  <h2 className="sec">Port</h2>
                  <Field label="MCP and UI port" hint="Loopback port for MCP, the API and this page. Changing it takes effect after restarting memtree; clients need their snippets updated.">
                    <NumberField label="MCP and UI port" value={draft.mcpPort} min={1024} max={65535} onChange={(n) => set('mcpPort', n)} />
                  </Field>
                </>
              )}

              {section === 'about' && (
                <>
                  <h1>About</h1>
                  <dl className="kv about">
                    <dt>version</dt><dd>{s.app?.version}</dd>
                    <dt>platform</dt><dd>{s.app?.platform}</dd>
                    <dt>config file</dt><dd>{s.app?.configPath}</dd>
                    <dt>database</dt><dd><Database size={12} /> {saved.dbPath || 'default location (set MEMTREE_DB to override)'}</dd>
                    <dt>mcp</dt><dd>{s.app?.mcp.running ? s.app.mcp.url : s.app?.mcp.error || 'not running'}</dd>
                    <dt>axis roots</dt><dd>{(s.app?.axisRoots ?? []).join(', ')}</dd>
                  </dl>
                  <p className="muted">memtree runs no model. Agents summarise and classify; memtree stores, ranks, forgets gradually, and shows it all here.</p>
                </>
              )}
            </div>
          </div>

          {dirty && (
            <div className="insp-foot settings-foot" role="region" aria-label="Unsaved settings">
              <span className={`grow ${problems.length ? '' : 'faint'}`} style={problems.length ? { color: 'var(--err)' } : undefined}>
                {problems[0] ?? (needsRestart(saved, draft) ? 'Unsaved changes. The port change needs a restart to apply.' : 'Unsaved changes.')}
              </span>
              <button className="btn sm" onClick={() => setDraft(saved)}>Revert</button>
              <button className="btn primary sm" disabled={problems.length > 0} onClick={async () => {
                const ok = await s.saveSettings(draft);
                if (ok) s.toast({ kind: 'ok', title: 'Settings saved', body: needsRestart(saved, draft) ? 'Restart memtree to use the new port.' : undefined });
              }}>
                Save settings
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
