import { Suspense } from 'react';

import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createRouterTransport } from '@connectrpc/connect';
import { render, screen } from '@test-utils';

import { ClusterStatus_State, StatusService } from '@/gen/status/v3alpha/status_pb';

import { ConfigPage } from './Config.page';

const transport = createRouterTransport(({ service }) => {
  service(StatusService, {
    getStatus: () => ({
      status: {
        versionInfo: {},
        config: { original: 'route:\n  receiver: default\n' },
        cluster: { state: ClusterStatus_State.DISABLED },
        startTime: timestampFromDate(new Date('2026-01-02T03:04:05.000Z')),
      },
    }),
  });
});

describe('ConfigPage', () => {
  it('renders configuration from the v3alpha service', async () => {
    render(
      <Suspense fallback="Loading">
        <ConfigPage />
      </Suspense>,
      { transport }
    );

    expect(await screen.findByText(/receiver: default/)).toBeInTheDocument();
  });
});
