import { useAPIQuery } from '@/data/api';

export type Group = {
  alerts: Alert[];
  labels: Record<string, string>;
  receiver: Receiver;
};

export type Receiver = {
  name: string;
};

export type AlertStatus = {
  inhibitedBy: string[];
  silencedBy: string[];
  mutedBy: string[];
  state: 'active';
};

export type Alert = {
  annotations: Record<string, string>;
  endsAt: string;
  fingerprint: string;
  receivers: Receiver[];
  startsAt: string;
  status: AlertStatus;
  updatedAt: string;
  labels: Record<string, string>;
};

export type useGroupParams = {
  silenced?: 'true' | 'false';
  inhibited?: 'true' | 'false';
  filter?: Record<string, string>;
};

export const useGroups = (params?: useGroupParams) => {
  const filterEntries = params?.filter
    ? Object.entries(params.filter).map(([key, value]) => `${key}="${value}"`)
    : [];
  return useAPIQuery<Array<Group>>({
    path: '/alerts/groups',
    params: {
      silenced: params?.silenced ?? 'false',
      inhibited: params?.inhibited ?? 'false',
      ...(filterEntries.length > 0 ? { filter: filterEntries } : {}),
    },
  });
};
