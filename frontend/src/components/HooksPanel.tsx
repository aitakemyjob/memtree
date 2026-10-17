import { useState } from 'react';
import { Check, FolderOpen, Play, Trash2, X } from 'lucide-react';
import { api } from '../lib/api';
import { useStore } from '../lib/store';
import type { HookResult, HarnessStatus } from '../lib/types';
import { ErrorBox, Spinner, useAsync } from './ui';

/** Whether a harness needs a project directory before it can be installed. */
export function needsProject(harness: string): boolean {
  return harness === 'vscode';
}

const ACTION_CLASS: Record<string, string> = { add: 'ok', update: 'accent', remove: 'warn', run: 'violet', note: '' };

/** Shows what an install/uninstall/dry-run did (or would do). */
export function ResultBox({ result }: { result: HookResult }) {
  return (
    <div className="result-box" role="status">
      <div className="row" style={{ marginBottom: 6 }}>
        <b>{result.dryRun ? 'Dry run: nothing was written' : 'Done'}</b>
        <span className="faint">{result.harness}</span>
      </div>
      {(result.changes ?? []).length === 0 && <div className="muted">No changes were needed.</div>}
      {(result.changes ?? []).map((c, i) => (
        <div key={i} className="change-row">
          <span className={`badge ${ACTION_CLASS[c.action] ?? ''}`}>{c.action}</span>
          <span className="mono grow ellipsis" title={c.file}>{c.file}</span>
          <span className="faint ellipsis" title={c.detail}>{c.detail}</span>
        </div>
      ))}
      {(result.notes ?? []).map((n, i) => (
        <div key={i} className="muted" style={{ marginTop: 6 }}>{n}</div>
      ))}
    </div>
  );
}

/**
 * Per-harness hook and MCP installer. One shared project path applies to
 * every card (VS Code requires it; the others install globally without it).
 */
export function HooksPanel() {
  const s = useStore();
  const canPick = s.app?.canPickDir ?? false;
  const [project, setProject] = useState('');
  const [tick, setTick] = useState(0);
  const [busy, setBusy] = useState<string>();
  const [results, setResults] = useState<Record<string, HookResult>>({});
  const status = useAsync(() => api.hooksStatus(), [tick]);

  const run = async (h: HarnessStatus, mode: 'install' | 'uninstall' | 'dry') => {
    setBusy(`${h.harness}:${mode}`);
    try {
      const r = mode === 'uninstall' ? await api.uninstallHooks(h.harness, project) : await api.installHooks(h.harness, project, mode === 'dry');
      setResults((x) => ({ ...x, [h.harness]: r }));
      if (mode !== 'dry') {
        setTick((t) => t + 1);
        s.toast({ kind: 'ok', title: `${mode === 'install' ? 'Installed' : 'Uninstalled'} ${h.name}`, body: 'Restart the harness to pick up the change.' });
      }
    } catch (e) {
      s.fail(`${h.name}: ${mode} failed`, e);
    } finally {
      setBusy(undefined);
    }
  };

  const pick = async () => {
    try {
      const r = await api.pickDirectory();
      if (r.path) setProject(r.path);
    } catch (e) {
      s.fail('Could not open the folder picker', e);
    }
  };

  const harnesses = status.data?.harnesses ?? [];
  return (
    <div className="col" style={{ gap: 14 }}>
      <div className="muted">
        Hooks let a harness recall memories on every prompt and record what happened; the MCP entry lets it call memtree's tools. Installing merges into the harness's own config and keeps what is already there.
      </div>
      <div className="col" style={{ gap: 6 }}>
        <label className="field-label" htmlFor="hook-project">Project folder (optional)</label>
        <div className="row">
          <input id="hook-project" className="input mono" placeholder="/path/to/project; required for VS Code" value={project} onChange={(e) => setProject(e.target.value)} />
          {canPick && (
            <button className="btn" onClick={pick}>
              <FolderOpen size={14} /> Choose…
            </button>
          )}
        </div>
        <div className="field-hint">Leave empty to install for your user account. With a folder, Claude Code and Cursor are configured for that project only and VS Code (always per workspace) needs one. The ticks below reflect your user-level setup.</div>
      </div>
      {status.loading && !status.data && <div style={{ position: 'relative', height: 80 }}><Spinner /></div>}
      <ErrorBox error={status.error} />
      {status.data && <div className="faint">Hook command: <span className="mono">{status.data.exe}</span></div>}
      {harnesses.map((h) => {
        const missingProject = needsProject(h.harness) && !project.trim();
        const r = results[h.harness];
        return (
          <div key={h.harness} className="card harness">
            <div className="row" style={{ marginBottom: 8 }}>
              <b style={{ fontSize: 14 }}>{h.name}</b>
              <span className={`badge ${h.detected ? 'ok' : ''}`}>{h.detected ? 'detected' : 'not detected'}</span>
              <span className="grow" />
              <span className={`status-flag ${h.hooks ? 'on' : ''}`}>{h.hooks ? <Check size={13} /> : <X size={13} />} hooks</span>
              <span className={`status-flag ${h.mcp ? 'on' : ''}`}>{h.mcp ? <Check size={13} /> : <X size={13} />} MCP</span>
            </div>
            {h.limits && <div className="muted" style={{ marginBottom: 8 }}>{h.limits}</div>}
            {(h.files ?? []).length > 0 && (
              <div className="faint mono" style={{ marginBottom: 8, wordBreak: 'break-all' }}>
                {(h.files ?? []).map((f) => <div key={f}>{f}</div>)}
              </div>
            )}
            {(h.events ?? []).length > 0 && (
              <div className="chips" style={{ marginBottom: 10 }}>
                {(h.events ?? []).map((e) => <span key={e} className="chip">{e}</span>)}
              </div>
            )}
            <div className="row" style={{ flexWrap: 'wrap' }}>
              <button className="btn primary sm" disabled={!!busy || missingProject} onClick={() => run(h, 'install')} title={missingProject ? 'Choose a project folder first' : undefined}>
                {busy === `${h.harness}:install` ? 'Installing…' : h.hooks && h.mcp ? 'Reinstall' : 'Install'}
              </button>
              <button className="btn sm" disabled={!!busy || missingProject} onClick={() => run(h, 'dry')}>
                <Play size={12} /> Dry run
              </button>
              <button className="btn sm danger" disabled={!!busy || missingProject} onClick={() => run(h, 'uninstall')}>
                <Trash2 size={12} /> Uninstall
              </button>
              {missingProject && <span className="faint">Needs a project folder</span>}
            </div>
            {r && <ResultBox result={r} />}
          </div>
        );
      })}
    </div>
  );
}
