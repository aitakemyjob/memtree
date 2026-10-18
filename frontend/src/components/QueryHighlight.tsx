import { highlightQuery } from '../lib/traceExplain';
import type { TraceMatch } from '../lib/types';

/** The query or prompt, with each token that fired a lattice category marked and labelled. */
export function QueryHighlight({ query, matches }: { query: string; matches: TraceMatch[] }) {
  if (!query) return <span className="faint">(no query: injected at session start)</span>;
  return (
    <span className="query-hl">
      {highlightQuery(query, matches).map((seg, i) =>
        seg.categories.length ? (
          <mark key={i} className="qh-mark" title={`fired ${seg.categories.join(', ')}`}>{seg.text}</mark>
        ) : (
          <span key={i}>{seg.text}</span>
        ),
      )}
    </span>
  );
}

/** The token → category pairs a query fired. */
export function MatchList({ matches }: { matches: TraceMatch[] }) {
  if (!matches.length) return <span className="faint">No lattice categories fired.</span>;
  return (
    <div className="chips" aria-label="Fired categories">
      {matches.map((m) => (
        <span key={`${m.token}→${m.category}`} className="chip mono">{m.token} → {m.category}</span>
      ))}
    </div>
  );
}
