import { useSuspenseQuery } from '@connectrpc/connect-query';

import {
  type AlertmanagerStatus,
  ClusterStatus_State,
  type GetStatusResponse,
  StatusService,
} from '@/gen/status/v3alpha/status_pb';

type Status = AlertmanagerStatus & {
  versionInfo: NonNullable<AlertmanagerStatus['versionInfo']>;
  config: NonNullable<AlertmanagerStatus['config']>;
  cluster: NonNullable<AlertmanagerStatus['cluster']>;
  startTime: NonNullable<AlertmanagerStatus['startTime']>;
};

export const statusFromResponse = (response: GetStatusResponse): Status => {
  const { status } = response;
  if (
    status === undefined ||
    status.versionInfo === undefined ||
    status.config === undefined ||
    status.cluster === undefined ||
    status.startTime === undefined
  ) {
    throw new Error('GetStatus response is missing required status fields');
  }

  return status as Status;
};

export const formatClusterState = (state: ClusterStatus_State) => {
  switch (state) {
    case ClusterStatus_State.DISABLED:
      return 'disabled';
    case ClusterStatus_State.SETTLING:
      return 'settling';
    case ClusterStatus_State.READY:
      return 'ready';
    case ClusterStatus_State.UNSPECIFIED:
      return 'unspecified';
    default:
      return 'unknown';
  }
};

export const useStatus = () =>
  useSuspenseQuery(
    StatusService.method.getStatus,
    {},
    {
      select: statusFromResponse,
      retry: false,
      refetchOnWindowFocus: false,
      gcTime: 0,
    }
  );
