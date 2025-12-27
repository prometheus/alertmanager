import { useAPIQuery } from '@/data/api';

type Silence = {
  id: string;
  status: {
    state: 'active' | 'expired' | 'pending';
  };
  startsAt: string;
  updatedAt: string;
  endsAt: string;
  createdBy: string;
  comment: string;
  matchers: Array<{
    name: string;
    value: string;
    isRegex: boolean;
    isEqual: boolean;
  }>;
};

export const useSilences = () => {
  return useAPIQuery<Array<Silence>>({
    path: '/silences',
  });
};

export const useSilence = (id: string) => {
  return useAPIQuery<Silence>({
    path: `/silence/${id}`,
  });
};
