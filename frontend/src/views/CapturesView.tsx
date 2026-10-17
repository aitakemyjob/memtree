import { useEffect, useMemo, useState } from 'react';
import { CheckCircle2, CircleDot, Clock, MessagesSquare, Square } from 'lucide-react';
import { api } from '../lib/api';
import { ALL_PROJECTS } from '../lib/filters';
import { relTime, projectName, plural } from '../lib/format';
import { summarisePrompt } from '../lib/snippets';
import { useStore } from '../lib/store';
import type { CaptureSession } from '../lib/types';
import { captureState, sortCaptures, type CaptureState } from '../lib/captures';
import { CopyButton, Empty, ErrorBox, Spinner, useAsync } from '../components/ui';

const STATE_LABEL: Record<CaptureState, string> = { awaiting: 'needs summary', recording: 'recording', summarised: 'summarised' };

/**
 * Captured conversation sessions: the list (those waiting for a summary
 * first) and a chat-style transcript with a ready prompt that asks an
 * agent to summarise the session over MCP.
 */
export function CapturesView() {
  const s = useStore();
  const project = s.filters.project;
  const sessions = useAsync(() => api.captures(500), [s.versions.capture]);
  const [sel, setSel] = useState<string>();
  const list = useMemo(() => sortCaptures((sessions.data ?? []).filter((c) => project === ALL_PROJECTS || c.project === project || c.project === '')), [sessions.data, project]);
  const current = list.find((c) => c.id === sel) ?? list[0];
  useEffect(() => {
    if (current && current.id !== sel) setSel(current.id);
  }, [current, sel]);

  if (sessions.loading && !sessions.data) return <div className="view"><Spinner /></div>;
  return (
    <div className="view">
      <div className="toolbar">
        <span className="title serif">Captures</span>
        <span className="faint">{plural(list.length, 'session')}, {list.filter((c) => captureState(c) === 'awaiting').length} waiting for a summary</span>
        <span className="grow" />
        {!s.app?.settings.captureHooks && <span className="badge warn" title="Enable in Settings > Agent access">capture hooks are off</span>}
      </div>
      <ErrorBox error={sessions.error} />
      {list.length === 0 ? (
        <Empty icon={<MessagesSquare size={28} />} title="No captured sessions">
          With hooks installed, each conversation is recorded turn by turn. When a session ends it waits here for an agent to summarise it into a lasting memory.
          <div style={{ marginTop: 12 }}>
            <button className="btn primary" onClick={() => s.openView('settings', { section: 'hooks' })}>Install hooks</button>
          </div>
        </Empty>
      ) : (
        <div className="split">
          <div className="side" style={{ width: 320 }}>
            {list.map((c) => {
              const st = captureState(c);
              return (
                <button key={c.id} className={`list-item cap-item ${current?.id === c.id ? 'on' : ''}`} style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left' }} onClick={() => setSel(c.id)} aria-pressed={current?.id === c.id}>
                  <span className={`cap-icon ${st}`}>{st === 'summarised' ? <CheckCircle2 size={15} /> : st === 'recording' ? <CircleDot size={15} /> : <Clock size={15} />}</span>
                  <div className="grow" style={{ minWidth: 0 }}>
                    <div className="title ellipsis">{projectName(c.project)} <span className="faint">· {c.harness}</span></div>
                    <div className="sub">{plural(c.turns, 'turn')} · {relTime(c.lastAt)} · {STATE_LABEL[st]}</div>
                  </div>
                </button>
              );
            })}
          </div>
          <div className="main">{current && <Transcript key={current.id} session={current} />}</div>
        </div>
      )}
    </div>
  );
}

function Transcript({ session }: { session: CaptureSession }) {
  const s = useStore();
  const st = captureState(session);
  const turns = useAsync(() => api.turns(session.id), [session.id, s.versions.capture]);
  // The raw capture memory points at its summary once an agent has written one.
  const raw = useAsync(() => (session.recordId ? api.getMemory(session.recordId).catch(() => undefined) : Promise.resolve(undefined)), [session.recordId, s.versions.memory]);
  const summaryId = raw.data?.supersededBy;
  const [busy, setBusy] = useState(false);

  const close = async () => {
    setBusy(true);
    try {
      await api.closeCapture(session.id);
      s.bump({ capture: true, memory: true });
    } catch (e) {
      s.fail('Could not close the session', e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="cap-head">
        <div className="row" style={{ flexWrap: 'wrap' }}>
          <b className="serif" style={{ fontSize: 15 }}>{projectName(session.project)}</b>
          <span className="chip">{session.harness}</span>
          <span className={`badge ${st === 'awaiting' ? 'warn' : st === 'summarised' ? 'ok' : 'accent'}`}>{STATE_LABEL[st]}</span>
          <span className="faint">{plural(session.turns, 'turn')} · started {relTime(session.startedAt)}{session.closeReason ? ` · closed: ${session.closeReason}` : ''}</span>
          <span className="grow" />
          {st === 'recording' && <button className="btn sm" disabled={busy} onClick={close}><Square size={12} /> Close session</button>}
          {st !== 'summarised' && <CopyButton text={summarisePrompt(session)} label="Copy summarise prompt" className="" />}
        </div>
        <div className="row faint" style={{ marginTop: 6, flexWrap: 'wrap' }}>
          <span className="mono" title="Session id">{session.id}</span>
          {session.recordId && <button className="link" onClick={() => s.inspectMemory(session.recordId)}>raw capture memory</button>}
          {summaryId && <button className="link" onClick={() => s.inspectMemory(summaryId)}>summary memory</button>}
        </div>
        {st === 'awaiting' && (
          <div className="muted" style={{ marginTop: 8 }}>
            Paste the copied prompt into a connected agent. It reads this transcript with <span className="mono">capture_transcript</span> and saves a <span className="mono">memory_save</span> of kind <span className="mono">session</span> carrying this session id, which retires the raw capture.
          </div>
        )}
      </div>
      <div className="scroll grow transcript" style={{ position: 'relative' }}>
        {turns.loading && !turns.data && <Spinner />}
        <ErrorBox error={turns.error} />
        {(turns.data ?? []).map((t) => (
          <div key={t.seq} className={`bubble ${t.role === 'user' ? 'user' : 'assistant'}`}>
            <div className="bubble-meta">{t.role} · #{t.seq} · {relTime(t.at)}</div>
            <div className="bubble-text">{t.text}</div>
          </div>
        ))}
        {!turns.loading && (turns.data ?? []).length === 0 && <div className="faint" style={{ padding: 16 }}>No turns were recorded for this session.</div>}
      </div>
    </>
  );
}
