import { createClient } from '@connectrpc/connect';

import { StatusService } from '@/gen/status/v3alpha/status_pb';

import { createAPITransport, getAPIBaseURL } from './connect';

describe('getAPIBaseURL', () => {
  it.each([
    ['https://example.com/ui/#/status', 'https://example.com/api'],
    ['https://example.com/alertmanager/ui/#/status', 'https://example.com/alertmanager/api'],
    [
      'https://example.com/monitoring/alertmanager/ui/#/status',
      'https://example.com/monitoring/alertmanager/api',
    ],
  ])('resolves %s to %s', (baseURI, expected) => {
    expect(getAPIBaseURL(baseURI)).toBe(expected);
  });
});

describe('createAPITransport', () => {
  it('calls GetStatus with Connect HTTP GET under the route prefix', async () => {
    let request: Request | undefined;
    const mockFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      request = new Request(input, init);
      return new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    };
    const client = createClient(
      StatusService,
      createAPITransport('https://example.com/alertmanager/ui/#/status', mockFetch)
    );

    await client.getStatus({});

    expect(request).toBeDefined();
    expect(request?.method).toBe('GET');
    const url = new URL(request!.url);
    expect(url.pathname).toBe('/alertmanager/api/status.v3alpha.StatusService/GetStatus');
    expect(url.searchParams.get('connect')).toBe('v1');
    expect(url.searchParams.get('encoding')).toBe('json');
    expect(url.searchParams.get('message')).toBe('{}');
  });
});
