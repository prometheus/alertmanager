import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import { ClusterStatus_State, GetStatusResponseSchema } from '@/gen/status/v3alpha/status_pb';

import { formatClusterState, statusFromResponse } from './status';

const createStatusResponse = () =>
  create(GetStatusResponseSchema, {
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
        peers: [],
      },
      startTime: timestampFromDate(new Date('2026-01-02T03:04:05.000Z')),
    },
  });

describe('statusFromResponse', () => {
  it('unwraps a complete status response', () => {
    const response = createStatusResponse();

    expect(statusFromResponse(response)).toBe(response.status);
  });

  it('rejects a missing status', () => {
    expect(() => statusFromResponse(create(GetStatusResponseSchema))).toThrow(
      'GetStatus response is missing required status fields'
    );
  });

  it.each(['versionInfo', 'config', 'cluster', 'startTime'] as const)(
    'rejects a missing %s field',
    (field) => {
      const response = createStatusResponse();
      response.status![field] = undefined;

      expect(() => statusFromResponse(response)).toThrow(
        'GetStatus response is missing required status fields'
      );
    }
  );
});

describe('formatClusterState', () => {
  it.each([
    [ClusterStatus_State.DISABLED, 'disabled'],
    [ClusterStatus_State.SETTLING, 'settling'],
    [ClusterStatus_State.READY, 'ready'],
    [ClusterStatus_State.UNSPECIFIED, 'unspecified'],
    [99 as ClusterStatus_State, 'unknown'],
  ])('formats %s as %s', (state, expected) => {
    expect(formatClusterState(state)).toBe(expected);
  });
});
