// Decides what a backend bus event means for the UI. Pure (no store, no
// timers) so every rule is unit-tested; the store applies the result.
import type { Activity, BusEvent, MCPStatus, Settings } from './types';

export interface EventContext {
  /** Capture session ids the UI has already seen, so only new ones toast. */
  knownSessions: ReadonlySet<string>;
}

export interface ToastPlan {
  kind: 'info' | 'ok' | 'warn' | 'err' | 'agent';
  title: string;
  body?: string;
  /** What the toast's button opens. */
  open?: 'latest-memory' | 'captures';
}

export interface EventEffects {
  /** Which data sets are stale and need a (debounced) refetch. */
  stale: { memory?: boolean; lattice?: boolean; capture?: boolean; trace?: boolean };
  /** Whether app.state (counts) should be refreshed too. */
  refreshStats: boolean;
  activity?: Activity;
  mcp?: MCPStatus;
  settings?: Settings;
  /** A memory id worth remembering as "latest saved" for toast actions. */
  savedMemory?: string;
  /** A capture session id newly seen. */
  newSession?: string;
  toast?: ToastPlan;
}

const SAVE_TOOLS = new Set(['memory_save', 'memory_compress', 'memory_supersede']);

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null;

/** Maps one bus event to its UI effects. Unknown or malformed events produce no effects. */
export function planEvent(ev: BusEvent, ctx: EventContext): EventEffects {
  const none: EventEffects = { stale: {}, refreshStats: false };
  const d = ev.data;
  switch (ev.type) {
    case 'config':
      return isObj(d) ? { ...none, settings: d as unknown as Settings } : none;
    case 'mcp':
      return isObj(d) ? { ...none, mcp: d as unknown as MCPStatus } : none;
    case 'activity': {
      if (!isObj(d)) return none;
      const a = d as unknown as Activity;
      const fx: EventEffects = { ...none, activity: a };
      if (a.tool === 'connect' && a.source === 'mcp') {
        fx.toast = { kind: 'agent', title: a.summary || 'Agent connected', body: 'Connected over MCP' };
      } else if (a.source === 'mcp' && SAVE_TOOLS.has(a.tool)) {
        fx.toast = a.error
          ? { kind: 'err', title: `${a.harness || 'Agent'} could not ${a.tool.replace('memory_', '')}`, body: a.error }
          : { kind: 'agent', title: `${a.harness || 'Agent'} ${a.tool === 'memory_save' ? 'saved a memory' : a.tool === 'memory_compress' ? 'distilled memories' : 'superseded a memory'}`, body: a.summary, open: 'latest-memory' };
      } else if (a.error && a.source === 'hook') {
        fx.toast = { kind: 'warn', title: `Hook ${a.tool} failed`, body: a.error };
      }
      return fx;
    }
    case 'memory': {
      const id = isObj(d) && typeof d.id === 'string' ? d.id : undefined;
      const op = isObj(d) && typeof d.op === 'string' ? d.op : '';
      return { ...none, stale: { memory: true }, refreshStats: true, ...(id && ['save', 'compress', 'capture'].includes(op) ? { savedMemory: id } : {}) };
    }
    case 'lattice':
      return { ...none, stale: { lattice: true, memory: true }, refreshStats: true };
    case 'trace':
      return { ...none, stale: { trace: true } };
    case 'capture': {
      const session = isObj(d) && typeof d.session === 'string' ? d.session : '';
      const op = isObj(d) && typeof d.op === 'string' ? d.op : '';
      const fx: EventEffects = { ...none, stale: { capture: true }, refreshStats: op !== 'turn' };
      if (session && !ctx.knownSessions.has(session)) {
        fx.newSession = session;
        fx.toast = { kind: 'info', title: 'New session capture', body: `${session} is being recorded by a hook`, open: 'captures' };
      } else if (op === 'summarized') {
        fx.toast = { kind: 'ok', title: 'Session summarised', body: session, open: 'captures' };
      }
      return fx;
    }
    default:
      return none;
  }
}
