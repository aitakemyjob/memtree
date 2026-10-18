import { Plug, Plus, Wrench } from 'lucide-react';
import { useStore } from '../lib/store';

/**
 * Shown when MemTree holds no memories yet: the three steps that get an
 * agent writing memory, each with a button that jumps to the right place.
 */
export function Welcome() {
  const s = useStore();
  return (
    <div className="welcome">
      <img src="./favicon.svg" alt="" width={44} height={44} />
      <h1>Give your coding agents a memory</h1>
      <p className="muted">
        MemTree keeps what your agents learn across sessions, files it on a category lattice, and lets it fade the way real memory does. You can read, correct and organise all of it here.
      </p>
      <ol className="steps">
        <li>
          <span className="num">1</span>
          <div className="grow">
            <b>Install hooks</b>
            <div className="muted">Hooks recall relevant memories on every prompt and record sessions for Claude Code, Codex, Cursor and VS Code.</div>
          </div>
          <button className="btn primary" onClick={() => s.openView('settings', { section: 'hooks' })}><Wrench size={14} /> Install hooks</button>
        </li>
        <li>
          <span className="num">2</span>
          <div className="grow">
            <b>Connect MCP</b>
            <div className="muted">Gives agents the recall and memory_save tools, so they can look things up and write decisions down.</div>
          </div>
          <button className="btn" onClick={() => s.setConnectOpen(true)}><Plug size={14} /> Connection snippets</button>
        </li>
        <li>
          <span className="num">3</span>
          <div className="grow">
            <b>Watch memory form</b>
            <div className="muted">Work as usual. Saved memories appear here live; check the Activity view to confirm hooks are firing, or add one yourself to see how it works.</div>
          </div>
          <button className="btn" onClick={() => s.newMemory()}><Plus size={14} /> New memory</button>
        </li>
      </ol>
    </div>
  );
}
