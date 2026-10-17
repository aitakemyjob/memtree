import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, BrowserTransport, desktopAvailable, getTransport, setTransport } from './transport';

afterEach(() => {
  setTransport(undefined);
  vi.unstubAllGlobals();
  delete (window as unknown as Record<string, unknown>).go;
  delete (window as unknown as Record<string, unknown>).runtime;
});

describe('BrowserTransport', () => {
  it('POSTs method and args with the CSRF header and unwraps result', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ result: { ok: true } }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const out = await new BrowserTransport().invoke('memory.pin', { id: 'm', pinned: true });
    expect(out).toEqual({ ok: true });
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/invoke');
    expect(init.method).toBe('POST');
    expect((init.headers as Record<string, string>)['X-Memtree']).toBe('1');
    expect(JSON.parse(init.body as string)).toEqual({ method: 'memory.pin', args: { id: 'm', pinned: true } });
  });
  it('sends null args when none are given', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ result: 1 }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await new BrowserTransport().invoke('app.state');
    expect(JSON.parse((fetchMock.mock.calls[0] as unknown as [string, RequestInit])[1].body as string)).toEqual({ method: 'app.state', args: null });
  });
  it('turns {error} bodies and HTTP failures into ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'memory not found' }), { status: 200 })));
    await expect(new BrowserTransport().invoke('memory.get', { id: 'x' })).rejects.toMatchObject({ message: 'memory not found', method: 'memory.get' });
    vi.stubGlobal('fetch', vi.fn(async () => new Response('missing X-Memtree header', { status: 403 })));
    await expect(new BrowserTransport().invoke('app.state')).rejects.toBeInstanceOf(ApiError);
  });
  it('subscribes over SSE and ignores malformed frames', () => {
    const sources: { onmessage?: (m: { data: string }) => void; close: () => void; url: string }[] = [];
    vi.stubGlobal('EventSource', class {
      onmessage?: (m: { data: string }) => void;
      close = vi.fn();
      constructor(public url: string) {
        sources.push(this);
      }
    });
    const t = new BrowserTransport();
    const got: unknown[] = [];
    const off = t.subscribe((e) => got.push(e));
    expect(sources).toHaveLength(1);
    expect(sources[0].url).toBe('/api/events?gd=1');
    sources[0].onmessage!({ data: '{"type":"memory","data":{"id":"a","op":"save"}}' });
    sources[0].onmessage!({ data: 'not json' });
    expect(got).toEqual([{ type: 'memory', data: { id: 'a', op: 'save' } }]);
    off();
    expect(sources[0].close).toHaveBeenCalled();
  });
});

describe('transport selection', () => {
  it('falls back to the browser transport when Wails is absent', async () => {
    expect(desktopAvailable()).toBe(false);
    expect((await getTransport()).kind).toBe('browser');
  });
  it('uses Wails bindings (Invoke with a JSON string, mt:event) when present', async () => {
    const invoke = vi.fn(async () => '{"v":1}');
    const handlers: Record<string, (d: unknown) => void> = {};
    Object.assign(window, {
      go: { main: { Desktop: { Invoke: invoke } } },
      runtime: { EventsOn: (name: string, cb: (d: unknown) => void) => { handlers[name] = cb; return () => {}; } },
    });
    const t = await getTransport();
    expect(t.kind).toBe('desktop');
    expect(await t.invoke('app.state', { a: 1 })).toEqual({ v: 1 });
    expect(invoke).toHaveBeenCalledWith('app.state', '{"a":1}');
    const got: unknown[] = [];
    t.subscribe((e) => got.push(e));
    handlers['mt:event']({ type: 'mcp' });
    expect(got).toEqual([{ type: 'mcp' }]);
  });
  it('maps a rejected Wails call (string error) to ApiError', async () => {
    Object.assign(window, { go: { main: { Desktop: { Invoke: vi.fn(async () => { throw 'unknown method'; }) } } } });
    await expect((await getTransport()).invoke('nope')).rejects.toMatchObject({ message: 'unknown method' });
  });
});
