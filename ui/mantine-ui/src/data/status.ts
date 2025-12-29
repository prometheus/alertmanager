import { useSuspenseAPIQuery } from '@/data/api';

type Status = {
  cluster: {
    name: string;
    peers: Array<{
      name: string;
      address: string;
    }>;
    status: 'ready' | 'not_ready';
  };
  config: {
    original: string;
  };
  uptime: string;
  versionInfo: {
    branch: string;
    buildDate: string;
    buildUser: string;
    goVersion: string;
    version: string;
    revision: string;
  };
};

export const useStatus = () => {
  return useSuspenseAPIQuery<Status>({
    path: '/status',
  });
};
