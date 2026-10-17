// Opt-in parity check against a live backend: the TypeScript activation port
// must agree with the Go service. Run with a scratch database:
//   MEMTREE_PARITY_URL=http://127.0.0.1:47493 npm test -- activation.parity
import { describe, expect, it } from 'vitest';
import { compressInDays, memoryActivation, nodeActivation } from './activation';
import type { CategoryNode, Explained, MemoryRecord, TierConfig } from './types';

const base = process.env.MEMTREE_PARITY_URL;
const DAY = 86_400_000;

async function call<T>(method: string, args?: unknown): Promise<T> {
  const r = await fetch(`${base}/api/invoke`, { method: 'POST', headers: { 'X-Memtree': '1', 'Content-Type': 'application/json' }, body: JSON.stringify({ method, args }) });
  const j = (await r.json()) as { result?: T; error?: string };
  if (j.error) throw new Error(j.error);
  return j.result as T;
}

describe.skipIf(!base)('activation parity with the Go backend', () => {
  const ago = (d: number) => new Date(Date.now() - d * DAY).toISOString();
  const cases: { name: string; rec: Partial<MemoryRecord> }[] = [
    { name: 'fresh', rec: { createdAt: ago(0) } },
    { name: 'one day', rec: { createdAt: ago(1), accessLog: [ago(1)] } },
    { name: 'a week episodic', rec: { createdAt: ago(7), accessLog: [ago(7)] } },
    { name: 'a quarter episodic', rec: { createdAt: ago(90), accessLog: [ago(90)] } },
    { name: 'semantic 40 days', rec: { tier: 'semantic', createdAt: ago(40), accessLog: [ago(40)] } },
    { name: 'many accesses', rec: { createdAt: ago(60), accessLog: [ago(60), ago(30), ago(10), ago(2), ago(0.01)] } },
    { name: 'sub-hour access', rec: { createdAt: ago(0.01), accessLog: [ago(0.01)] } },
  ];
  for (const c of cases) {
    it(`matches Go for ${c.name}`, async () => {
      const saved = await call<MemoryRecord>('memory.save', { summary: `parity ${c.name} ${Math.random()}`, kind: 'note', ...c.rec });
      const ex = await call<Explained>('memory.explain', { id: saved.id });
      const tiers = (await call<{ settings: { tiers: TierConfig } }>('app.state')).settings.tiers;
      const now = Date.now();
      expect(memoryActivation(ex.record, now)).toBeCloseTo(ex.activation, 3);
      expect(compressInDays(ex.record, tiers, now) === 0).toBe(ex.compressible);
    });
  }
  it('matches Go for pinned and for lattice nodes', async () => {
    const saved = await call<MemoryRecord>('memory.save', { summary: `parity pinned ${Math.random()}`, kind: 'note', pinned: true, createdAt: ago(200), accessLog: [ago(200)] });
    const ex = await call<Explained>('memory.explain', { id: saved.id });
    expect(ex.activation).toBe(1);
    expect(memoryActivation(ex.record, Date.now())).toBe(1);
    const nodes = await call<CategoryNode[]>('lattice.nodes');
    expect(nodes.length).toBeGreaterThan(0);
    for (const n of nodes) expect(nodeActivation(n.lastAccessed, Date.now())).toBeCloseTo(n.activation, 3);
  });
});
