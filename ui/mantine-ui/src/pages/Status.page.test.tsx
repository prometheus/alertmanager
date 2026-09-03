import { Suspense } from 'react';

import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createRouterTransport } from '@connectrpc/connect';
import { render, screen, within } from '@test-utils';

import { ClusterStatus_State, StatusService } from '@/gen/status/v3alpha/status_pb';

import { StatusPage } from './Status.page';

const transport = createRouterTransport(({ service }) => {
  service(StatusService, {
    getStatus: () => ({
      status: {
        versionInfo: {
          version: '1.2.3',
          revision: 'abc123',
          branch: 'main',
          buildUser: 'builder',
          buildDate: '2026-01-02',
          goVersion: 'go1.26',
        },
        config: { original: 'route:\n  receiver: default\n' },
        cluster: {
          name: 'test-cluster',
          state: ClusterStatus_State.READY,
          peers: [{ name: 'node-a', address: '10.0.0.1:9094' }],
        },
        startTime: timestampFromDate(new Date('2026-01-02T03:04:05.000Z')),
      },
    }),
  });
});

describe('StatusPage', () => {
  it('renders status from the v3alpha service', async () => {
    render(
      <Suspense fallback="Loading">
        <StatusPage />
      </Suspense>,
      { transport }
    );

    expect(await screen.findByText('1.2.3')).toBeInTheDocument();
    expect(screen.getByText('abc123')).toBeInTheDocument();
    expect(screen.getByText('main')).toBeInTheDocument();
    expect(screen.getByText('builder')).toBeInTheDocument();
    expect(screen.getByText('2026-01-02')).toBeInTheDocument();
    expect(screen.getByText('go1.26')).toBeInTheDocument();
    expect(screen.getByText('Start Time')).toBeInTheDocument();
    expect(screen.queryByText('Uptime')).not.toBeInTheDocument();
    expect(screen.getByText('2026-01-02T03:04:05.000Z')).toBeInTheDocument();
    expect(screen.getByText('test-cluster')).toBeInTheDocument();
    expect(screen.getByText('ready')).toBeInTheDocument();
    expect(screen.getByText('node-a')).toBeInTheDocument();
    expect(screen.getByText('10.0.0.1:9094')).toBeInTheDocument();

    const peerCountRow = screen.getByText('Number of Peers').closest('tr');
    expect(peerCountRow).not.toBeNull();
    expect(within(peerCountRow!).getByText('1')).toBeInTheDocument();
  });
});
