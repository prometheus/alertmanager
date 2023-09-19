import { useQuery } from '@tanstack/react-query';
import buildURL from '../utils/url-builder';
import { fetchJson } from '../utils/fetch';

const resource = 'status';

export interface AMStatusClusterPeersInfo {
  address: string;
  name: string;
}

export interface AMStatusClusterInfo {
  name: string;
  peers: AMStatusClusterPeersInfo[];
  status: string;
}

export interface AMStatusVersionInfo {
  branch: string;
  buildDate: string;
  buildUser: string;
  goVersion: string;
  revision: string;
  version: string;
}

export interface AMStatus {
  cluster: AMStatusClusterInfo;
  uptime: string;
  versionInfo: AMStatusVersionInfo;
  config: {
    original: string;
  };
}

export function useAMStatus() {
  return useQuery<AMStatus, Error>([], () => {
    const url = buildURL({ resource: resource });
    return fetchJson<AMStatus>(url);
  });
}
