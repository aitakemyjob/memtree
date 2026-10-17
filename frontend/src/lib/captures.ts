import type { CaptureSession } from './types';

/** Where a capture session is in its life. */
export type CaptureState = 'awaiting' | 'recording' | 'summarised';

/** awaiting = closed with no summary yet, recording = still open, summarised = done. */
export function captureState(c: CaptureSession): CaptureState {
  if (c.summarizedAt) return 'summarised';
  return c.closedAt ? 'awaiting' : 'recording';
}

/** Sessions needing a summary first, then still-recording ones, then done; newest first within each. */
export function sortCaptures(list: CaptureSession[]): CaptureSession[] {
  const order: Record<CaptureState, number> = { awaiting: 0, recording: 1, summarised: 2 };
  return [...list].sort((a, b) => order[captureState(a)] - order[captureState(b)] || Date.parse(b.lastAt) - Date.parse(a.lastAt));
}
