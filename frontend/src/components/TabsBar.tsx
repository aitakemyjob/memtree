import { X } from 'lucide-react';
import { useStore } from '../lib/store';
import { VIEW_ICONS } from './ActivityBar';

/** Tab strip. Middle-click closes, double-click closes the others. */
export function TabsBar() {
  const s = useStore();
  return (
    <div className="tabs" role="tablist">
      {s.tabs.map((t) => {
        const Icon = VIEW_ICONS[t.kind];
        return (
          <div
            key={t.id}
            role="tab"
            tabIndex={0}
            aria-selected={t.id === s.active}
            className={`tab ${t.id === s.active ? 'on' : ''}`}
            onClick={() => s.activate(t.id)}
            onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && s.activate(t.id)}
            onAuxClick={(e) => e.button === 1 && s.close(t.id)}
            onDoubleClick={() => s.closeOthers(t.id)}
            title={`${t.title}: double-click to close others`}
          >
            <Icon size={14} style={{ flex: 'none', opacity: 0.8 }} />
            <span className="ellipsis">{t.title}</span>
            <button
              className="x"
              onClick={(e) => {
                e.stopPropagation();
                s.close(t.id);
              }}
              aria-label={`Close ${t.title}`}
            >
              <X size={12} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
