import { useSuspenseAPIQuery } from '@/data/api';

type Group = {
  alerts: Alert[];
  labels: Record<string, string>;
  receiver: Receiver;
};

type Receiver = {
  name: string;
};

type AlertStatus = {
  inhibitedBy: string[];
  silencedBy: string[];
  mutedBy: string[];
  state: 'active';
};

type Alert = {
  annotations: Record<string, string>;
  endsAt: string;
  fingerprint: string;
  receivers: Receiver[];
  startsAt: string;
  status: AlertStatus;
  updatedAt: string;
  labels: Record<string, string>;
};

export const useGroups = () => {
  return useSuspenseAPIQuery<Array<Group>>({
    path: '/alerts/groups',
  });
};
