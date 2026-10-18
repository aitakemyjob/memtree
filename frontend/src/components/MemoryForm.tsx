import { IMPORTANCE_LEVELS } from '../lib/format';
import { projectName } from '../lib/format';
import { knownKinds } from '../lib/filters';
import { useStore } from '../lib/store';
import type { MemoryDraft } from '../lib/types';
import { Field, PathEditor, StringList } from './ui';

const PROVENANCE = [
  { value: 'HUMAN_CONFIRMED', label: 'Human confirmed', hint: 'You checked it' },
  { value: 'SOURCE_GROUNDED', label: 'Source grounded', hint: 'Taken from code or docs' },
  { value: 'MODEL_INFERRED', label: 'Model inferred', hint: 'An agent concluded it' },
];

/**
 * The shared editor for creating and editing a memory: summary, keywords,
 * decisions, open loops, importance, kind, project, provenance and lattice
 * paths. Controlled: the caller owns the draft.
 */
export function MemoryForm({ draft, onChange, isNew }: { draft: MemoryDraft; onChange: (d: MemoryDraft) => void; isNew?: boolean }) {
  const projects = useStore((s) => s.app?.projects ?? []);
  const set = <K extends keyof MemoryDraft>(k: K, v: MemoryDraft[K]) => onChange({ ...draft, [k]: v });
  const known = projects.some((p) => p.project === draft.project);
  return (
    <div className="col form" style={{ gap: 14 }}>
      <Field label="Summary" hint="One self-contained statement. Say why, not just what.">
        <textarea className="input" aria-label="Summary" rows={4} value={draft.summary} onChange={(e) => set('summary', e.target.value)} placeholder="e.g. We use SQLite with WAL because the service is single-writer and local." />
      </Field>

      <Field label="Importance" hint={IMPORTANCE_LEVELS.find((l) => l.value === draft.importance)?.hint}>
        <div className="seg" role="group" aria-label="Importance">
          {IMPORTANCE_LEVELS.map((l) => (
            <button key={l.value} className={draft.importance === l.value ? 'on' : ''} aria-pressed={draft.importance === l.value} onClick={() => set('importance', l.value)} title={`${l.value}x ranking weight`}>
              {l.name} <span className="faint mono">{l.value}</span>
            </button>
          ))}
        </div>
      </Field>

      <div className="grid2">
        <Field label="Kind">
          <input className="input" aria-label="Kind" list="mt-kinds" value={draft.kind} onChange={(e) => set('kind', e.target.value)} />
          <datalist id="mt-kinds">{knownKinds([]).map((k) => <option key={k} value={k} />)}</datalist>
        </Field>
        <Field label="Project">
          <select className="input select" aria-label="Project" value={draft.project} onChange={(e) => set('project', e.target.value)}>
            <option value="">Global (all projects)</option>
            {!known && draft.project && <option value={draft.project}>{projectName(draft.project)}</option>}
            {projects.filter((p) => p.project !== '').map((p) => <option key={p.project} value={p.project}>{projectName(p.project)}</option>)}
          </select>
        </Field>
      </div>

      {isNew && (
        <Field label="Tier" hint="Semantic memories are distilled gist and fade slowly.">
          <div className="seg" role="group" aria-label="Tier">
            {(['episodic', 'semantic'] as const).map((t) => (
              <button key={t} className={draft.tier === t ? 'on' : ''} aria-pressed={draft.tier === t} onClick={() => set('tier', t)}>{t}</button>
            ))}
          </div>
        </Field>
      )}

      <Field label="Provenance" hint={PROVENANCE.find((p) => p.value === draft.provenance)?.hint}>
        <select className="input select" aria-label="Provenance" value={draft.provenance} onChange={(e) => set('provenance', e.target.value)}>
          {PROVENANCE.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
          {!PROVENANCE.some((p) => p.value === draft.provenance) && <option value={draft.provenance}>{draft.provenance}</option>}
        </select>
      </Field>

      <Field label="Lattice categories" hint="Where this sits in the category lattice. Saved as human-placed links.">
        <PathEditor paths={draft.latticePaths} onChange={(p) => set('latticePaths', p)} />
      </Field>

      <Field label="Keywords" hint="Used by text search. Leave empty to have MemTree extract them.">
        <StringList label="keyword" values={draft.keywords} onChange={(v) => set('keywords', v)} placeholder="Add a keyword and press Enter" />
      </Field>

      <Field label="Decisions">
        <StringList label="decision" multiline values={draft.decisions} onChange={(v) => set('decisions', v)} placeholder="Add a decision and press Enter" />
      </Field>

      <Field label="Open loops" hint="Things still unresolved.">
        <StringList label="open loop" multiline values={draft.openLoops} onChange={(v) => set('openLoops', v)} placeholder="Add an open loop and press Enter" />
      </Field>
    </div>
  );
}
