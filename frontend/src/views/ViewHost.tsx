import { useStore } from '../lib/store';
import type { TabKind } from '../lib/tabs';
import { ActivityView } from './ActivityView';
import { CapturesView } from './CapturesView';
import { ConsolidationView } from './ConsolidationView';
import { LatticeView } from './LatticeView';
import { MemoriesView } from './MemoriesView';
import { SettingsView } from './SettingsView';
import { TimelineView } from './TimelineView';

const views: Record<TabKind, () => React.JSX.Element | null> = {
  memories: MemoriesView,
  lattice: LatticeView,
  timeline: TimelineView,
  captures: CapturesView,
  consolidation: ConsolidationView,
  activity: ActivityView,
  settings: SettingsView,
};

/** Renders the active tab's view (only one is mounted at a time). */
export function ViewHost() {
  const tab = useStore((s) => s.tabs.find((t) => t.id === s.active));
  if (!tab) return <div className="view" />;
  const View = views[tab.kind];
  return <View key={tab.id} />;
}
