import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Background, Controls, Handle, Position, ReactFlow, ReactFlowProvider, useReactFlow, type Node, type NodeProps } from '@xyflow/react';
import { ChevronDown, ChevronUp, FolderPlus, GitMerge, Maximize, Radar, Scissors, Search, Tag, X } from 'lucide-react';
import { api } from '../lib/api';
import { buildFlow, emphasisMap, nodeSize, type LatticeNodeData } from '../lib/latticeFlow';
import { layout } from '../lib/layout';
import { fixed, relTime, truncate } from '../lib/format';
import { useStore } from '../lib/store';
import type { FireResult, UntaggedItem } from '../lib/types';
import { LintIcon, LintPanel } from '../components/LintPanel';
import { useDebounced } from '../lib/hooks';
import { Confirm, Empty, ErrorBox, Meter, Modal, PathEditor, PathInput, Spinner, useAsync } from '../components/ui';

const CategoryNodeView = memo(function CategoryNodeView({ data }: NodeProps<Node<LatticeNodeData>>) {
  const e = data.emphasis;
  return (
    <div className={`cat-node ${data.axisRoot ? 'axis' : ''} e-${e}`} style={{ width: data.width, height: data.height, opacity: e === 'dim' ? 0.22 : data.opacity }} title={`${data.id}\n${data.linkCount} linked, activation ${data.activation.toFixed(2)}`}>
      <Handle type="target" position={Position.Left} />
      <span className="cat-label ellipsis">{data.label}</span>
      {data.linkCount > 0 && <span className="cat-count">{data.linkCount}</span>}
      <Handle type="source" position={Position.Right} />
    </div>
  );
});
const nodeTypes = { category: CategoryNodeView };

/**
 * The category lattice as an interactive DAG. Node size is how many
 * memories hang off it, opacity is its current activation, axis roots are
 * ringed. The Probe box runs lattice.fire: type a prompt and see which
 * categories it lights up and which memories would surface.
 */
export function LatticeView() {
  return (
    <ReactFlowProvider>
      <LatticeInner />
    </ReactFlowProvider>
  );
}

/** Stable empty fallback so memoised layouts don't recompute before app state loads. */
const NO_AXES: string[] = [];

function LatticeInner() {
  const s = useStore();
  const rf = useReactFlow();
  const nodes = s.nodes;
  const axisRoots = s.app?.axisRoots ?? NO_AXES;
  const selected = s.inspect?.kind === 'node' ? s.inspect.id : undefined;

  const [search, setSearch] = useState('');
  const [probeText, setProbeText] = useState('');
  const [fire, setFire] = useState<FireResult>();
  const [probing, setProbing] = useState(false);
  const [positions, setPositions] = useState<Map<string, { x: number; y: number }>>(new Map());
  const [merge, setMerge] = useState(false);
  const [create, setCreate] = useState(false);
  const [prune, setPrune] = useState(false);
  const [queueOpen, setQueueOpen] = useState(false);
  const [lintOpen, setLintOpen] = useState(false);
  const [focus, setFocus] = useState<string[]>([]);
  const [lintTick, setLintTick] = useState(0);
  const loaded = useRef(false);

  // ELK only needs to re-run when the shape of the tree changes, not when
  // activation or emphasis does.
  const structure = useMemo(() => nodes.map((n) => `${n.id}|${n.parentId}|${nodeSize(n.label, n.linkCount).width}x${nodeSize(n.label, n.linkCount).height}`).join('\n'), [nodes]);
  useEffect(() => {
    let alive = true;
    const sized = nodes.map((n) => ({ id: n.id, ...nodeSize(n.label, n.linkCount) }));
    const edges = nodes.filter((n) => n.parentId).map((n) => ({ id: `${n.parentId}->${n.id}`, source: n.parentId, target: n.id }));
    layout(sized, edges, 'RIGHT', { spacing: 14, layerSpacing: 56 }).then((p) => {
      if (!alive) return;
      setPositions(p);
      if (!loaded.current) {
        loaded.current = true;
        setTimeout(() => rf.fitView({ padding: 0.12, duration: 0 }), 30);
      }
    }, () => {});
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [structure]);

  const runProbe = useCallback(async (text: string) => {
    const t = text.trim();
    if (!t) {
      setFire(undefined);
      return;
    }
    setProbing(true);
    try {
      setFire(await api.fire(t, 20));
    } catch (e) {
      useStore.getState().fail('Probe failed', e);
    } finally {
      setProbing(false);
    }
  }, []);

  // A probe requested from the command palette.
  const req = s.probeRequest;
  useEffect(() => {
    if (req) {
      setProbeText(req.text);
      void runProbe(req.text);
    }
  }, [req, runProbe]);

  const flow = useMemo(() => {
    const em = emphasisMap(nodes, { seeds: fire?.seeds ?? [], hits: fire?.hits ?? [], selected, query: search, focus });
    return buildFlow(nodes, axisRoots, em, positions);
  }, [nodes, axisRoots, fire, selected, search, focus, positions]);

  const hits = fire?.hits ?? [];
  const untagged = useAsync(() => api.untagged(300), [s.versions.lattice, s.versions.memory]);
  // Lint refetches (debounced) on lattice/memory events and after any fix.
  const lintKey = useDebounced(`${s.versions.lattice}:${s.versions.memory}:${lintTick}`, 300);
  const lint = useAsync(() => api.lint(), [lintKey]);
  const showNodes = (ids: string[]) => {
    setFocus(ids);
    s.inspectNode(ids[0]);
    setTimeout(() => rf.fitView({ nodes: ids.map((id) => ({ id })), padding: 0.6, duration: 300, maxZoom: 1.2 }), 30);
  };
  const pending = untagged.data?.length ?? s.app?.stats.untagged ?? 0;

  return (
    <div className="view">
      <div className="toolbar lattice-toolbar">
        <div className="probe-box">
          <Radar size={15} style={{ color: 'var(--accent-text)' }} />
          <input
            className="search-input"
            aria-label="Probe the lattice"
            placeholder="Probe: type a prompt to see what it lights up…"
            value={probeText}
            onChange={(e) => setProbeText(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && void runProbe(probeText)}
          />
          {fire && (
            <button className="btn ghost icon sm" aria-label="Clear probe" onClick={() => { setFire(undefined); setProbeText(''); }}>
              <X size={13} />
            </button>
          )}
          <button className="btn primary sm" onClick={() => void runProbe(probeText)} disabled={probing || !probeText.trim()}>
            {probing ? 'Probing…' : 'Probe'}
          </button>
        </div>
        <div className="search-box" style={{ maxWidth: 220 }}>
          <Search size={14} className="faint" />
          <input className="search-input" aria-label="Find a category" placeholder="Find category…" value={search} onChange={(e) => setSearch(e.target.value)} />
        </div>
        <span className="grow" />
        <button className="btn sm" onClick={() => rf.fitView({ padding: 0.12, duration: 250 })} title="Fit to view"><Maximize size={13} /> Fit</button>
        <button className="btn sm" onClick={() => setCreate(true)}><FolderPlus size={13} /> New category</button>
        <button className="btn sm" disabled={!selected} onClick={() => setMerge(true)} title={selected ? `Merge ${selected} into another category` : 'Select a category first'}><GitMerge size={13} /> Merge</button>
        <button className={`btn sm ${lintOpen ? 'active' : ''}`} onClick={() => setLintOpen((o) => !o)} aria-expanded={lintOpen} title="Lattice hygiene findings">
          <LintIcon size={13} /> Lint {lint.data && lint.data.total > 0 && <span className="badge warn" aria-label={`${lint.data.total} findings`}>{lint.data.total}</span>}
        </button>
        <button className="btn sm" onClick={() => setPrune(true)} title="Remove empty leaf categories"><Scissors size={13} /> Prune empty</button>
      </div>

      <div className="graph-host">
        {nodes.length === 0 ? (
          <Empty icon={<Radar size={28} />} title="The lattice is empty">
            Categories appear as agents file memories under paths like project/architecture/db. You can also create one yourself.
          </Empty>
        ) : (
          <ReactFlow
            nodes={flow.nodes}
            edges={flow.edges}
            nodeTypes={nodeTypes}
            colorMode={s.theme.mode}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable
            minZoom={0.1}
            maxZoom={2}
            proOptions={{ hideAttribution: true }}
            onNodeClick={(_, n) => s.inspectNode(n.id)}
            onPaneClick={() => selected && s.closeInspector()}
            aria-label="Category lattice"
          >
            <Background gap={22} size={1} color="var(--grid)" />
            <Controls showInteractive={false} />
          </ReactFlow>
        )}

        <div className="graph-legend">
          <span><i className="lg axis" /> axis root</span>
          <span><i className="lg" /> category (size = linked memories, fade = cooler)</span>
          {fire && <span><i className="lg seed" /> fired</span>}
          {fire && <span><i className="lg via" /> surfaced memories</span>}
        </div>

        {fire && (
          <div className="probe-panel" role="region" aria-label="Probe result">
            <div className="row" style={{ marginBottom: 6 }}>
              <b className="serif">Probe result</b>
              <span className="grow" />
              <button className="btn ghost icon sm" aria-label="Close probe result" onClick={() => setFire(undefined)}><X size={13} /></button>
            </div>
            <div className="faint" style={{ marginBottom: 4 }}>Categories fired by the text</div>
            <div className="chips" style={{ marginBottom: 10 }}>
              {fire.seeds.length === 0 && <span className="muted">None. Nothing in the text matches a category, so only wording search would find memories.</span>}
              {fire.seeds.map((c) => <button key={c} className="chip mono clickable seed" onClick={() => s.inspectNode(c)}>{c}</button>)}
            </div>
            <div className="faint" style={{ marginBottom: 4 }}>Memories that would surface ({hits.length})</div>
            <div className="probe-hits">
              {hits.length === 0 && <div className="muted">No memories reached the activation threshold.</div>}
              {hits.map((h) => (
                <button key={h.itemId} className="link-row" onClick={() => s.inspectMemory(h.itemId)}>
                  <div className="row">
                    <Meter value={h.score} color="var(--accent-text)" width={44} label="Hit score" />
                    <span className="mono faint">{fixed(h.score)}</span>
                    <span className="chip mono">{h.viaCategory}</span>
                  </div>
                  <div className="link-summary">{truncate(h.summary || h.itemId, 130)}</div>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      {lintOpen && (
        <div className="queue open lint-panel" role="region" aria-label="Lattice lint">
          <div className="queue-head" style={{ cursor: 'default' }}>
            <LintIcon size={14} />
            <b>Lattice lint</b>
            <span className={`badge ${lint.data?.total ? 'warn' : 'ok'}`}>{lint.data?.total ?? 0}</span>
            <span className="faint grow">Advisory findings. Nothing changes until you press a button.</span>
            {focus.length > 0 && <button className="btn sm" onClick={() => setFocus([])}>Clear highlight</button>}
            <button className="btn ghost icon sm" aria-label="Close lint panel" onClick={() => { setLintOpen(false); setFocus([]); }}><X size={13} /></button>
          </div>
          <div className="queue-body" style={{ maxHeight: 320 }}>
            <LintPanel report={lint.data} loading={lint.loading} error={lint.error} onShow={showNodes} onChanged={() => setLintTick((t) => t + 1)} />
          </div>
        </div>
      )}

      <div className={`queue ${queueOpen ? 'open' : ''}`}>
        <button className="queue-head" onClick={() => setQueueOpen((o) => !o)} aria-expanded={queueOpen}>
          <Tag size={14} />
          <b>Unclassified queue</b>
          <span className={`badge ${pending > 0 ? 'warn' : ''}`}>{pending}</span>
          <span className="faint grow" style={{ textAlign: 'left' }}>Memories waiting for a category. Agents file these over MCP; you can too.</span>
          {queueOpen ? <ChevronDown size={15} /> : <ChevronUp size={15} />}
        </button>
        {queueOpen && (
          <div className="queue-body">
            {untagged.loading && !untagged.data && <Spinner />}
            <ErrorBox error={untagged.error} />
            {(untagged.data ?? []).length === 0 && !untagged.loading && <div className="faint" style={{ padding: 12 }}>Everything is classified.</div>}
            {(untagged.data ?? []).map((u) => <UntaggedRow key={u.itemId} item={u} />)}
          </div>
        )}
      </div>

      {create && (
        <Modal title="New category" onClose={() => setCreate(false)}>
          <div className="muted">Creates the path and every ancestor. Start from an axis root ({axisRoots.join(', ')}) so agents find it.</div>
          <PathInput
            autoFocus
            buttonLabel="Create"
            placeholder="e.g. project/memtree/retrieval"
            label="New category path"
            onPick={async (p) => {
              try {
                const r = await api.ensure(p);
                s.toast({ kind: 'ok', title: 'Category ready', body: r.id });
                s.bump({ lattice: true });
                void s.refreshNodes();
                setCreate(false);
                s.inspectNode(r.id);
              } catch (e) {
                s.fail('Could not create the category', e);
              }
            }}
          />
        </Modal>
      )}
      {merge && selected && (
        <Modal title={`Merge ${selected}`} onClose={() => setMerge(false)}>
          <div className="muted">Moves every memory link from <span className="mono">{selected}</span> into the category you pick, then removes <span className="mono">{selected}</span>. Use it to tidy near-duplicates like project/db and project/database.</div>
          <PathInput
            autoFocus
            allowNew={false}
            exclude={[selected]}
            buttonLabel="Merge into"
            placeholder="Target category"
            label="Merge target"
            onPick={async (to) => {
              try {
                await api.merge(selected, to);
                s.toast({ kind: 'ok', title: 'Merged', body: `${selected} into ${to}` });
                s.bump({ lattice: true, memory: true });
                void s.refreshNodes();
                setMerge(false);
                s.inspectNode(to);
              } catch (e) {
                s.fail('Merge failed', e);
              }
            }}
          />
        </Modal>
      )}
      {prune && (
        <Confirm
          title="Prune empty categories?"
          confirmLabel="Prune"
          danger
          onClose={() => setPrune(false)}
          onConfirm={async () => {
            try {
              const r = await api.prune();
              s.toast({ kind: 'ok', title: r.removed === 0 ? 'Nothing to prune' : `Removed ${r.removed} empty ${r.removed === 1 ? 'category' : 'categories'}` });
              s.bump({ lattice: true });
              void s.refreshNodes();
            } catch (e) {
              s.fail('Prune failed', e);
            }
          }}
        >
          Removes leaf categories that have no memories linked, other than the built-in axis seeds. Memories are not affected.
        </Confirm>
      )}
    </div>
  );
}

function UntaggedRow({ item }: { item: UntaggedItem }) {
  const s = useStore();
  const [paths, setPaths] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const tag = async () => {
    setBusy(true);
    try {
      await api.tag(item.itemId, paths);
      s.toast({ kind: 'ok', title: 'Classified', body: paths.join(', ') });
      s.bump({ lattice: true, memory: true });
      void s.refreshNodes();
    } catch (e) {
      s.fail('Could not classify', e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="queue-row">
      <div className="grow" style={{ minWidth: 0 }}>
        <button className="link" style={{ textAlign: 'left' }} onClick={() => s.inspectMemory(item.itemId)}>{truncate(item.text, 150)}</button>
        <div className="faint">{item.itemId} · {relTime(item.at)}</div>
      </div>
      <div style={{ width: 340, flex: 'none' }}>
        <PathEditor paths={paths} onChange={setPaths} />
      </div>
      <button className="btn primary sm" disabled={paths.length === 0 || busy} onClick={tag}>Tag</button>
    </div>
  );
}
