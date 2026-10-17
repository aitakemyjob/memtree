import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Check, Copy, Plus, X } from 'lucide-react';
import { usePathWarnings } from '../lib/hooks';
import { applySuggestion } from '../lib/pathwarn';
import { suggestPaths } from '../lib/paths';
import { useStore } from '../lib/store';
import { clamp } from '../lib/format';

/** Dialog that closes on Escape or a backdrop click. */
export function Modal(props: { title: ReactNode; onClose: () => void; children: ReactNode; footer?: ReactNode; wide?: boolean; icon?: ReactNode }) {
  const { onClose } = props;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`modal ${props.wide ? 'wide' : ''}`} role="dialog" aria-modal="true">
        <div className="modal-head">
          {props.icon}
          <span className="grow serif">{props.title}</span>
          <button className="btn ghost icon sm" onClick={onClose} aria-label="Close">
            <X size={15} />
          </button>
        </div>
        <div className="modal-body">{props.children}</div>
        {props.footer && <div className="modal-foot">{props.footer}</div>}
      </div>
    </div>
  );
}

/** Confirmation dialog for destructive actions. */
export function Confirm(props: { title: string; children: ReactNode; confirmLabel: string; danger?: boolean; onConfirm: () => void; onClose: () => void }) {
  return (
    <Modal
      title={props.title}
      onClose={props.onClose}
      footer={
        <>
          <button className="btn" onClick={props.onClose}>Cancel</button>
          <button
            className={`btn ${props.danger ? 'danger' : 'primary'}`}
            autoFocus
            onClick={() => {
              props.onConfirm();
              props.onClose();
            }}
          >
            {props.confirmLabel}
          </button>
        </>
      }
    >
      {props.children}
    </Modal>
  );
}

/** Accessible on/off toggle. */
export function Switch({ on, onChange, label }: { on: boolean; onChange: (v: boolean) => void; label?: string }) {
  return <button className={`switch ${on ? 'on' : ''}`} role="switch" aria-checked={on} aria-label={label} onClick={() => onChange(!on)} />;
}

/** Centered loading spinner with an optional label. */
export function Spinner({ label }: { label?: string }) {
  return (
    <div className="loading">
      <div className="col" style={{ alignItems: 'center' }}>
        <div className="spinner" />
        {label && <span>{label}</span>}
      </div>
    </div>
  );
}

/** Empty-state placeholder with an icon, title and body. */
export function Empty({ icon, title, children }: { icon: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <div>
        <div className="hero-icon">{icon}</div>
        <h2>{title}</h2>
        <div style={{ maxWidth: 460, margin: '0 auto' }}>{children}</div>
      </div>
    </div>
  );
}

/** Renders an error message, or nothing when error is falsy. */
export function ErrorBox({ error }: { error: unknown }) {
  if (!error) return null;
  return <div className="error-box" role="alert">{error instanceof Error ? error.message : String(error)}</div>;
}

/** Copies text to the clipboard with brief confirmation. */
export function CopyButton({ text, label = 'Copy', className = 'copy' }: { text: string; label?: string; className?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      className={`btn sm ${className}`}
      onClick={() => {
        navigator.clipboard?.writeText(text).catch(() => {});
        setDone(true);
        setTimeout(() => setDone(false), 1200);
      }}
    >
      {done ? <Check size={13} /> : <Copy size={13} />} {done ? 'Copied' : label}
    </button>
  );
}

/** Monospace snippet with a copy button. */
export function Snippet({ text }: { text: string }) {
  return (
    <div className="snippet">
      {text}
      <CopyButton text={text} />
    </div>
  );
}

/** Segmented control. */
export function Seg<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: ReactNode; title?: string }[]; onChange: (v: T) => void; label?: string }) {
  return (
    <div className="seg" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} className={o.value === value ? 'on' : ''} aria-pressed={o.value === value} title={o.title} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** Thin horizontal bar for a 0..1 value. */
export function Meter({ value, color = 'var(--accent)', label, width = 56 }: { value: number; color?: string; label?: string; width?: number }) {
  const v = clamp(Number.isFinite(value) ? value : 0, 0, 1);
  return (
    <span className="meter" style={{ width }} role="meter" aria-valuemin={0} aria-valuemax={1} aria-valuenow={v} aria-label={label}>
      <i style={{ width: `${v * 100}%`, background: color }} />
    </span>
  );
}

/** Measures an element (for canvas-like views). */
export function useSize<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });
  useEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(([e]) => setSize({ width: e.contentRect.width, height: e.contentRect.height }));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return [ref, size] as const;
}

/**
 * Runs an async loader, exposing data/error/loading; reruns when deps
 * change. Stale results from superseded runs are dropped.
 */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [state, setState] = useState<{ data?: T; error?: unknown; loading: boolean }>({ loading: true });
  useEffect(() => {
    let alive = true;
    setState((s) => ({ data: s.data, loading: true }));
    fn().then(
      (data) => alive && setState({ data, loading: false }),
      (error) => alive && setState({ error, loading: false }),
    );
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return state;
}

/** Editable list of short strings (keywords, decisions, open loops). */
export function StringList({ values, onChange, placeholder, multiline, label }: { values: string[]; onChange: (v: string[]) => void; placeholder: string; multiline?: boolean; label: string }) {
  const [draft, setDraft] = useState('');
  const add = () => {
    const v = draft.trim();
    if (v && !values.includes(v)) onChange([...values, v]);
    setDraft('');
  };
  return (
    <div className="col" style={{ gap: 6 }}>
      {values.length > 0 && (
        <div className={multiline ? 'col' : 'chips'} style={multiline ? { gap: 4 } : undefined}>
          {values.map((v, i) =>
            multiline ? (
              <div key={i} className="list-edit">
                <textarea
                  className="input"
                  aria-label={`${label} ${i + 1}`}
                  rows={2}
                  value={v}
                  onChange={(e) => onChange(values.map((x, j) => (j === i ? e.target.value : x)))}
                />
                <button className="btn ghost icon sm" aria-label={`Remove ${label} ${i + 1}`} onClick={() => onChange(values.filter((_, j) => j !== i))}>
                  <X size={13} />
                </button>
              </div>
            ) : (
              <span key={v} className="chip">
                {v}
                <button aria-label={`Remove ${v}`} onClick={() => onChange(values.filter((x) => x !== v))}>
                  <X size={11} />
                </button>
              </span>
            ),
          )}
        </div>
      )}
      <div className="row">
        <input
          className="input"
          aria-label={`Add ${label}`}
          placeholder={placeholder}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              add();
            }
          }}
        />
        <button className="btn sm" onClick={add} disabled={!draft.trim()} aria-label={`Add ${label}`}>
          <Plus size={13} />
        </button>
      </div>
    </div>
  );
}

/**
 * Text input with a lattice-path autocomplete. The list renders in a
 * portal with fixed positioning so scrolling/overflow-hidden ancestors (the
 * inspector, modals) can never clip it.
 */
export function PathInput({ onPick, placeholder = 'category/path', exclude = [], allowNew = true, autoFocus, buttonLabel, label = 'Category path' }: {
  onPick: (path: string) => void;
  placeholder?: string;
  exclude?: string[];
  allowNew?: boolean;
  autoFocus?: boolean;
  buttonLabel?: ReactNode;
  label?: string;
}) {
  const nodes = useStore((s) => s.nodes);
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [sel, setSel] = useState(0);
  const [rect, setRect] = useState<{ left: number; top: number; width: number }>();
  const ref = useRef<HTMLInputElement>(null);
  const sugg = suggestPaths(nodes, text, 8, exclude).filter((x) => allowNew || !x.isNew);

  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const r = ref.current?.getBoundingClientRect();
      if (r) setRect({ left: r.left, top: r.bottom + 4, width: r.width });
    };
    place();
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => {
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    };
  }, [open, text]);

  const choose = (p: string) => {
    onPick(p);
    setText('');
    setSel(0);
  };
  const submit = () => {
    const pick = sugg[Math.min(sel, sugg.length - 1)];
    if (pick) choose(pick.path);
    else if (allowNew && text.trim()) choose(text);
  };

  return (
    <div className="row" style={{ position: 'relative' }}>
      <input
        ref={ref}
        className="input mono"
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
        aria-label={label}
        autoFocus={autoFocus}
        placeholder={placeholder}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setSel(0);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 120)}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            setOpen(true);
            setSel((x) => Math.min(sugg.length - 1, x + 1));
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setSel((x) => Math.max(0, x - 1));
          } else if (e.key === 'Enter') {
            e.preventDefault();
            submit();
          } else if (e.key === 'Escape' && open) {
            e.stopPropagation();
            setOpen(false);
          }
        }}
      />
      {buttonLabel && (
        <button className="btn sm" onClick={submit} disabled={!text.trim()}>
          {buttonLabel}
        </button>
      )}
      {open &&
        rect &&
        sugg.length > 0 &&
        createPortal(
          <div className="ac-list" role="listbox" style={{ position: 'fixed', left: rect.left, top: rect.top, width: Math.max(rect.width, 240), zIndex: 90 }}>
            {sugg.map((x, i) => (
              <div key={x.path} role="option" aria-selected={i === sel} className={`ac-item ${i === sel ? 'on' : ''}`} onMouseDown={(e) => { e.preventDefault(); choose(x.path); }} onMouseEnter={() => setSel(i)}>
                <span className="mono ellipsis">{x.path}</span>
                {x.isNew ? <span className="badge accent">new</span> : <span className="faint">{x.linkCount}</span>}
              </div>
            ))}
          </div>,
          document.body,
        )}
    </div>
  );
}

/** Removable category chips plus an autocomplete input to add more. */
export function PathEditor({ paths, onChange }: { paths: string[]; onChange: (p: string[]) => void }) {
  return (
    <div className="col" style={{ gap: 6 }}>
      {paths.length > 0 && (
        <div className="chips">
          {paths.map((p) => (
            <span key={p} className="chip mono">
              {p}
              <button aria-label={`Remove category ${p}`} onClick={() => onChange(paths.filter((x) => x !== p))}>
                <X size={11} />
              </button>
            </span>
          ))}
        </div>
      )}
      <PathInput exclude={paths} onPick={(p) => !paths.includes(p) && onChange([...paths, p])} placeholder="Add a category, e.g. project/architecture/db" label="Add category" />
      <PathWarnings paths={paths} onChange={onChange} />
    </div>
  );
}

/** Labelled form field. */
export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="field">
      <label className="field-label">{label}</label>
      {children}
      {hint && <div className="field-hint">{hint}</div>}
    </div>
  );
}

/** Advisory warnings for the chosen paths, each with a one-click "Use <suggestion>". Never blocks anything. */
export function PathWarnings({ paths, onChange }: { paths: string[]; onChange: (p: string[]) => void }) {
  const warnings = usePathWarnings(paths);
  if (warnings.length === 0) return null;
  return (
    <div className="path-warnings" role="status" aria-label="Category warnings">
      {warnings.map((w, i) => (
        <div key={`${w.path}-${i}`} className="path-warning">
          <span className="mono">{w.path}</span>: {w.message}
          {w.suggest && (
            <button className="btn sm" onClick={() => onChange(applySuggestion(paths, w))}>
              Use <span className="mono">{w.suggest}</span>
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
