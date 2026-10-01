import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

import { API_PATH, createQueryFn } from './api';

// ---------------------------------------------------------------------------
// Live test server
// ---------------------------------------------------------------------------

let server: Server;
let serverPort: number;
let currentHandler: (req: IncomingMessage, res: ServerResponse) => void = (_req, res) => {
  res.writeHead(404);
  res.end();
};

const serverPrefix = () => `http://localhost:${serverPort}`;

beforeAll(async () => {
  server = createServer((req, res) => currentHandler(req, res));
  await new Promise<void>((resolve) => {
    server.listen(0, () => {
      serverPort = (server.address() as AddressInfo).port;
      resolve();
    });
  });
});

afterAll(async () => {
  await new Promise<void>((resolve, reject) =>
    server.close((err) => (err ? reject(err) : resolve()))
  );
});

function setHandler(fn: (req: IncomingMessage, res: ServerResponse) => void) {
  currentHandler = fn;
}

function jsonResponse(res: ServerResponse, status: number, body: unknown) {
  const payload = JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(payload);
}
// ---------------------------------------------------------------------------
// createQueryFn tests
// ---------------------------------------------------------------------------

describe('createQueryFn', () => {
  function qfn(
    path: string,
    params?: Record<string, string | string[]>,
    recordResponseTime?: (t: number) => void
  ) {
    return createQueryFn({
      pathPrefix: serverPrefix(),
      path,
      params,
      recordResponseTime,
    });
  }

  function signal() {
    return new AbortController().signal;
  }

  it('returns data from a successful API envelope', async () => {
    setHandler((_req, res) => jsonResponse(res, 200, { status: 'success', data: { id: 1 } }));
    const result = await qfn('/test')({ signal: signal() });
    expect(result).toEqual({ id: 1 });
  });

  it('returns raw JSON when the response is not an API envelope', async () => {
    setHandler((_req, res) => jsonResponse(res, 200, [{ id: 1 }, { id: 2 }]));
    const result = await qfn('/test')({ signal: signal() });
    expect(result).toEqual([{ id: 1 }, { id: 2 }]);
  });

  it('returns raw JSON for objects that lack a status field', async () => {
    setHandler((_req, res) => jsonResponse(res, 200, { foo: 'bar' }));
    const result = await qfn('/test')({ signal: signal() });
    expect(result).toEqual({ foo: 'bar' });
  });

  it('throws the error field from an API error envelope', async () => {
    setHandler((_req, res) =>
      jsonResponse(res, 200, { status: 'error', error: 'something went wrong' })
    );
    await expect(qfn('/test')({ signal: signal() })).rejects.toThrow('something went wrong');
  });

  it('throws a fallback message when the error envelope lacks the error field', async () => {
    setHandler((_req, res) => jsonResponse(res, 200, { status: 'error' }));
    await expect(qfn('/test')({ signal: signal() })).rejects.toThrow(
      'missing "error" field in response JSON'
    );
  });

  it('throws statusText for non-OK responses with a non-JSON content type', async () => {
    setHandler((_req, res) => {
      res.writeHead(503, 'Service Unavailable', { 'Content-Type': 'text/plain' });
      res.end('Starting up...');
    });
    await expect(qfn('/test')({ signal: signal() })).rejects.toThrow('Service Unavailable');
  });

  it('parses a JSON error envelope for non-OK responses with application/json content type', async () => {
    setHandler((_req, res) =>
      jsonResponse(res, 422, { status: 'error', error: 'invalid request' })
    );
    await expect(qfn('/test')({ signal: signal() })).rejects.toThrow('invalid request');
  });

  it('throws "Invalid JSON response" for malformed JSON bodies', async () => {
    setHandler((_req, res) => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end('this is not json {{{');
    });
    await expect(qfn('/test')({ signal: signal() })).rejects.toThrow('Invalid JSON response');
  });

  it('throws "Network error..." when the server is unreachable', async () => {
    // Port 1 requires root on Linux and is never open in test environments.
    const unreachable = createQueryFn({ pathPrefix: 'http://localhost:0', path: '/test' });
    await expect(unreachable({ signal: signal() })).rejects.toThrow(
      'Network error or unable to reach the server'
    );
  });

  it('propagates AbortError when the signal is already aborted', async () => {
    const controller = new AbortController();
    controller.abort();
    const fn = createQueryFn({ pathPrefix: serverPrefix(), path: '/test' });
    await expect(fn({ signal: controller.signal })).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('sends no query string when params is undefined', async () => {
    let capturedUrl = '';
    setHandler((req, res) => {
      capturedUrl = req.url ?? '';
      jsonResponse(res, 200, { status: 'success', data: null });
    });
    await qfn('/alerts')({ signal: signal() });
    expect(capturedUrl).toBe(`/${API_PATH}/alerts`);
  });

  it('appends single-value query params to the URL', async () => {
    let capturedUrl = '';
    setHandler((req, res) => {
      capturedUrl = req.url ?? '';
      jsonResponse(res, 200, { status: 'success', data: null });
    });
    await qfn('/alerts', { filter: 'active', severity: 'critical' })({ signal: signal() });
    const params = new URLSearchParams(capturedUrl.split('?')[1]);
    expect(params.get('filter')).toBe('active');
    expect(params.get('severity')).toBe('critical');
  });

  it('appends repeated keys for array query params', async () => {
    let capturedUrl = '';
    setHandler((req, res) => {
      capturedUrl = req.url ?? '';
      jsonResponse(res, 200, { status: 'success', data: null });
    });
    await qfn('/alerts', { matchers: ['severity=critical', 'env=prod'] })({ signal: signal() });
    const params = new URLSearchParams(capturedUrl.split('?')[1]);
    expect(params.getAll('matchers')).toEqual(['severity=critical', 'env=prod']);
  });

  it('builds the URL with API_PATH between pathPrefix and path', async () => {
    let capturedUrl = '';
    setHandler((req, res) => {
      capturedUrl = req.url ?? '';
      jsonResponse(res, 200, { status: 'success', data: null });
    });
    await qfn('/silences')({ signal: signal() });
    expect(capturedUrl).toBe(`/${API_PATH}/silences`);
  });

  it('calls recordResponseTime with a non-negative elapsed time on success', async () => {
    setHandler((_req, res) => jsonResponse(res, 200, { status: 'success', data: {} }));
    const times: number[] = [];
    await qfn('/test', undefined, (t) => times.push(t))({ signal: signal() });
    expect(times).toHaveLength(1);
    expect(times[0]).toBeGreaterThanOrEqual(0);
  });

  it('calls recordResponseTime even when the API envelope carries an error', async () => {
    // recordResponseTime fires after JSON is parsed, before the envelope error is thrown.
    setHandler((_req, res) => jsonResponse(res, 200, { status: 'error', error: 'oops' }));
    const times: number[] = [];
    await expect(
      qfn('/test', undefined, (t) => times.push(t))({ signal: signal() })
    ).rejects.toThrow('oops');
    expect(times).toHaveLength(1);
  });

  it('does not call recordResponseTime when the HTTP response is non-OK with non-JSON body', async () => {
    setHandler((_req, res) => {
      res.writeHead(503, 'Service Unavailable', { 'Content-Type': 'text/plain' });
      res.end('down');
    });
    const times: number[] = [];
    await expect(
      qfn('/test', undefined, (t) => times.push(t))({ signal: signal() })
    ).rejects.toThrow();
    expect(times).toHaveLength(0);
  });
});
