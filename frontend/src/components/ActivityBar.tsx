import { Activity, Combine, Footprints, LayoutGrid, MessagesSquare, Network, Plug, Settings, Brain, type LucideIcon } from 'lucide-react';
import { useStore } from '../lib/store';
import { VIEW_TITLES, type TabKind } from '../lib/tabs';

/** The views shown on the rail, in order. */
export const VIEW_ICONS: Record<TabKind, LucideIcon> = {
  memories: Brain,
  lattice: Network,
  timeline: LayoutGrid,
  traces: Footprints,
  captures: MessagesSquare,
  consolidation: Combine,
  activity: Activity,
  settings: Settings,
};

const views: TabKind[] = ['memories', 'lattice', 'timeline', 'traces', 'captures', 'consolidation', 'activity'];

/**
 * Left icon rail: one button per view (opens or focuses its tab), with a
 * badge for captures waiting for a summary. Connect and Settings sit at
 * the bottom.
 */
export function ActivityBar() {
  const s = useStore();
  const activeKind = s.tabs.find((t) => t.id === s.active)?.kind;
  const pending = s.app?.stats.capturesPending ?? 0;
  const untagged = s.app?.stats.untagged ?? 0;
  const badge = (k: TabKind) => (k === 'captures' ? pending : k === 'lattice' ? untagged : 0);
  return (
    <nav className="activitybar" aria-label="Views">
      {views.map((k) => {
        const Icon = VIEW_ICONS[k];
        const n = badge(k);
        return (
          <button
            key={k}
            className={`ab-btn ${activeKind === k ? 'on' : ''}`}
            title={VIEW_TITLES[k]}
            aria-label={VIEW_TITLES[k]}
            aria-current={activeKind === k ? 'page' : undefined}
            onClick={() => s.openView(k)}
          >
            <Icon size={19} strokeWidth={1.8} />
            {n > 0 && <span className="count">{n > 99 ? '99+' : n}</span>}
          </button>
        );
      })}
      <div className="bottom">
        <button className="ab-btn" title="Connect an agent" aria-label="Connect an agent" onClick={() => s.setConnectOpen(true)}>
          <Plug size={19} strokeWidth={1.8} />
        </button>
        <button
          className={`ab-btn ${activeKind === 'settings' ? 'on' : ''}`}
          title="Settings"
          aria-label="Settings"
          onClick={() => s.openView('settings')}
        >
          <Settings size={19} strokeWidth={1.8} />
        </button>
      </div>
    </nav>
  );
}
