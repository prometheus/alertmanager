import { useQuery } from '@tanstack/react-query';
import buildURL from '../utils/url-builder';
import { fetchJson } from '../utils/fetch';

const resource = 'alerts/groups';

export interface Receiver {
  name: string;
}

export interface AlertStatus {
  state: 'active' | 'unprocessed' | 'suppressed';
  inhibitedBy: string[];
  silenceBy: string[];
}

export interface Alert {
  labels: Record<string, string>;
  annotations: Record<string, string>;
  generatorURL?: string;
  receivers: Receiver[];
  fingerprint: string;
  startsAt: string;
  endsAt: string;
  updatedAt: string;
  status: AlertStatus;
}

export interface AlertGroup {
  labels: Record<string, string>;
  receiver: Receiver;
  alerts: Alert[];
}

export function useAlertGroups() {
  return useQuery<AlertGroup[], Error>([resource], () => {
    const url = buildURL({ resource: resource });
    return fetchJson<AlertGroup[]>(url);
  });
}
