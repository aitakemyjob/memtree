// Palette matching.

/**
 * Scores how well q matches text: substring hits rank highest, mostly
 * adjacent subsequences next, scattered matches score 0.
 */
export function fuzzyScore(text: string, q: string): number {
  if (!q) return 1;
  const t = text.toLowerCase();
  const query = q.toLowerCase();
  const idx = t.indexOf(query);
  if (idx >= 0) return 100 - idx - t.length / 50;
  let ti = 0;
  let score = 0;
  for (const ch of query) {
    const found = t.indexOf(ch, ti);
    if (found < 0) return 0;
    score += found === ti ? 3 : 1;
    ti = found + 1;
  }
  return score >= query.length * 2 ? score : 0;
}

