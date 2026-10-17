import { describe, expect, it } from 'vitest';
import { captureState, sortCaptures } from './captures';
import type { CaptureSession } from './types';

const s = (id: string, over: Partial<CaptureSession> = {}): CaptureSession => ({
  id, harness: 'claude-code', project: '', startedAt: '2026-03-01T00:00:00Z', lastAt: '2026-03-01T01:00:00Z', closeReason: '', recordId: '', turns: 3, ...over,
});

describe('captureState', () => {
  it('derives the state from closed/summarised timestamps', () => {
    expect(captureState(s('a'))).toBe('recording');
    expect(captureState(s('a', { closedAt: '2026-03-01T02:00:00Z' }))).toBe('awaiting');
    expect(captureState(s('a', { closedAt: '2026-03-01T02:00:00Z', summarizedAt: '2026-03-01T03:00:00Z' }))).toBe('summarised');
  });
});

describe('sortCaptures', () => {
  it('puts sessions that need a summary first, then recording, then summarised; newest first inside each', () => {
    const closed = '2026-03-02T00:00:00Z';
    const list = [
      s('done', { closedAt: closed, summarizedAt: closed, lastAt: '2026-03-05T00:00:00Z' }),
      s('live', { lastAt: '2026-03-04T00:00:00Z' }),
      s('old-await', { closedAt: closed, lastAt: '2026-03-01T00:00:00Z' }),
      s('new-await', { closedAt: closed, lastAt: '2026-03-03T00:00:00Z' }),
    ];
    expect(sortCaptures(list).map((x) => x.id)).toEqual(['new-await', 'old-await', 'live', 'done']);
  });
});
