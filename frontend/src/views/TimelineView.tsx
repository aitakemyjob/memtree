import { useMemo, useState } from 'react';
import { LayoutGrid } from 'lucide-react';
import { api } from '../lib/api';
import { filtersActive, listArgs, defaultFilters } from '../lib/filters';
import { useNow } from '../lib/hooks';
import { importanceName, plural, projectName, relTime, truncate } from '../lib/format';
import { useStore } from '../lib/store';
import { heatColor, readableOn } from '../lib/themes';
import { useHeat } from '../lib/traces';
import { HEAT_MODES, layoutMemories, modeNeedsTraces, type Cell, type ColorBy, type SizeBy } from '../lib/treemap';
import { Empty, ErrorBox, Seg, Spinner, useAsync, useSize } from '../components/ui';

const COLOR_OPTIONS = (Object.keys(HEAT_MODES) as ColorBy[]).map((value) => ({ value, label: HEAT_MODES[value].label }));

/**
 * Heatmap of live memories: tiles grouped project, then kind; area is
 * importance (or equal), colour is any mode in HEAT_MODES (activation,
 * age, importance, or how often a memory was retrieved and used). Click a
 * tile to open the memory.
 */
export function TimelineView() {
  const s = useStore();
  const nowMs = useNow(60_000);
  const [colorBy, setColorBy] = useState<ColorBy>('activation');
  const [sizeBy, setSizeBy] = useState<SizeBy>('importance');
  const [tip, setTip] = useState<{ cell: Cell; x: number; y: number }>();
  const [ref, size] = useSize<HTMLDivElement>();
  // Superseded and archived memories are not part of the live picture, so the sidebar's
  // "include" flags and pinned-only do not apply here.
  const args = listArgs({ ...defaultFilters, project: s.filters.project, tier: s.filters.tier, kind: s.filters.kind }, 3000);
  const data = useAsync(() => api.listMemories(args), [JSON.stringify(args), s.versions.memory]);
  const heat = useHeat({}, modeNeedsTraces(colorBy));
  const cells = useMemo(
    () => layoutMemories(data.data ?? [], nowMs, size.width, size.height, { colorBy, sizeBy, traces: heat.data }),
    [data.data, nowMs, size.width, size.height, colorBy, sizeBy, heat.data],
  );
  const legend = HEAT_MODES[colorBy].legend;
  const selected = s.inspect?.kind === 'memory' ? s.inspect.id : undefined;
  const total = (data.data ?? []).length;

  return (
    <div className="view">
      <div className="toolbar">
        <span className="title serif">Timeline</span>
        <span className="faint">{plural(total, 'live memory', 'live memories')}{filtersActive(s.filters) ? ' (filtered)' : ''}</span>
        <span className="grow" />
        <span className="faint">Colour</span>
        <Seg<ColorBy>
          label="Colour by"
          value={colorBy}
          onChange={setColorBy}
          options={COLOR_OPTIONS}
        />
        <span className="faint">Size</span>
        <Seg<SizeBy> label="Size by" value={sizeBy} onChange={setSizeBy} options={[{ value: 'importance', label: 'Importance' }, { value: 'equal', label: 'Equal' }]} />
        <div className="row" aria-label="Legend" style={{ gap: 6 }}>
          <span className="faint">{legend[0]}</span>
          <span className="legend-bar" />
          <span className="faint">{legend[1]}</span>
        </div>
      </div>
      <div className="graph-host" ref={ref}>
        {data.loading && !data.data && <Spinner />}
        <ErrorBox error={data.error} />
        {!data.loading && total === 0 && (
          <Empty icon={<LayoutGrid size={28} />} title="No live memories in this scope">Memories appear here as tiles once agents or you save some. Archived and superseded memories are left out.</Empty>
        )}
        <div className="treemap" role="list" aria-label="Memory heatmap">
          {cells.map((c) =>
            c.type !== 'memory' ? (
              <div key={c.key} className={`tm-cell dir d${c.depth}`} style={{ left: c.x0, top: c.y0, width: c.x1 - c.x0, height: c.y1 - c.y0 }}>
                <span className="lbl">{c.name}</span>
              </div>
            ) : (
              <button
                key={c.key}
                role="listitem"
                className={`tm-cell file ${selected === c.id ? 'sel' : ''}`}
                aria-label={`${c.name} (${c.metric})`}
                style={{ left: c.x0, top: c.y0, width: c.x1 - c.x0, height: c.y1 - c.y0, background: heatColor(s.theme, c.heat), color: readableOn(heatColor(s.theme, c.heat)) }}
                onClick={() => c.id && s.inspectMemory(c.id)}
                onMouseMove={(e) => setTip({ cell: c, x: e.clientX, y: e.clientY })}
                onMouseLeave={() => setTip(undefined)}
                onFocus={(e) => { const r = e.currentTarget.getBoundingClientRect(); setTip({ cell: c, x: r.left + 12, y: r.bottom }); }}
                onBlur={() => setTip(undefined)}
              >
                {c.x1 - c.x0 > 54 && c.y1 - c.y0 > 22 && <span className="lbl">{c.name}</span>}
              </button>
            ),
          )}
        </div>
        {tip?.cell.record && (
          <div className="tooltip" style={{ left: Math.min(tip.x + 14, window.innerWidth - 400), top: Math.min(tip.y + 14, window.innerHeight - 140) }}>
            <div style={{ fontWeight: 600, marginBottom: 4 }}>{truncate(tip.cell.record.summary.split('\n')[0], 120)}</div>
            <div className="faint">
              {projectName(tip.cell.record.project)} · {tip.cell.record.kind} · {importanceName(tip.cell.record.importance)} · {relTime(tip.cell.record.createdAt, nowMs)}
            </div>
            <div className="mono">{tip.cell.metric}</div>
          </div>
        )}
      </div>
    </div>
  );
}
