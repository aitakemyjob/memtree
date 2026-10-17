import { useEffect } from 'react';
import { useStore } from './lib/store';
import { TitleBar } from './components/TitleBar';
import { ActivityBar } from './components/ActivityBar';
import { Sidebar } from './components/Sidebar';
import { TabsBar } from './components/TabsBar';
import { ViewHost } from './views/ViewHost';
import { StatusBar } from './components/StatusBar';
import { Inspector } from './components/Inspector';
import { CommandPalette } from './components/CommandPalette';
import { Toasts } from './components/Toasts';
import { ConnectAgent } from './components/ConnectAgent';
import { Spinner } from './components/ui';

/**
 * Root component: boots the store, binds global shortcuts and lays out
 * the title bar, activity bar, sidebar, tabs, inspector and status bar.
 */
export function App() {
  const s = useStore();

  useEffect(() => {
    useStore.getState().init();
  }, []);

  // The lattice feeds every path autocomplete, so keep it fresh.
  useEffect(() => {
    if (s.ready) void useStore.getState().refreshNodes();
  }, [s.ready, s.versions.lattice]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      const st = useStore.getState();
      const k = e.key.toLowerCase();
      if (k === 'k' || (k === 'p' && !e.shiftKey)) {
        e.preventDefault();
        st.setPalette(!st.palette);
      } else if (k === 'b') {
        e.preventDefault();
        st.toggleSidebar();
      } else if (k === 'i') {
        e.preventDefault();
        st.toggleInspector();
      } else if (k === 'n') {
        e.preventDefault();
        st.newMemory();
      } else if (k === 'w' && st.active) {
        e.preventDefault();
        st.close(st.active);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (!s.ready) {
    return (
      <div className="app">
        <Spinner label="Starting memtree…" />
      </div>
    );
  }
  if (s.fatal) {
    return (
      <div className="app" style={{ display: 'grid', placeItems: 'center' }}>
        <div className="error-box" style={{ maxWidth: 520 }}>
          <b>memtree could not reach its backend.</b>
          <div className="muted" style={{ marginTop: 6 }}>{s.fatal}</div>
          <button className="btn" style={{ marginTop: 10 }} onClick={() => location.reload()}>Retry</button>
        </div>
      </div>
    );
  }
  return (
    <div className="app">
      <TitleBar />
      <div className="workbench">
        <ActivityBar />
        {s.sidebarOpen && <Sidebar />}
        <div className={`center ${s.inspectorOpen ? 'with-inspector' : ''}`}>
          <TabsBar />
          <ViewHost />
        </div>
        {s.inspectorOpen && <Inspector />}
      </div>
      <StatusBar />
      <Toasts />
      {s.palette && <CommandPalette />}
      {s.connectOpen && <ConnectAgent />}
    </div>
  );
}
