import type { MessageInitShape } from '@bufbuild/protobuf';
import { createQueryOptions, useTransport } from '@connectrpc/connect-query';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { type ListSilencesResponse, SilenceService } from '@/gen/silence/v3alpha/silence_pb';

const listSilences = SilenceService.method.listSilences;
const cacheOptions = { staleTime: 30_000, gcTime: 60_000, retry: false };

export type SilenceListInput = MessageInitShape<typeof listSilences.input>;

export function useSilences(input: SilenceListInput) {
  const transport = useTransport();
  return useQuery({
    ...createQueryOptions(listSilences, input, { transport }),
    ...cacheOptions,
  });
}

export function useSilence(id: string, listInput: SilenceListInput) {
  const transport = useTransport();
  const client = useQueryClient();
  const scoped = createQueryOptions(listSilences, listInput, { transport });
  const findSilence = (response?: ListSilencesResponse) =>
    response?.silences.find((silence) => silence.id === id);
  const cached = findSilence(client.getQueryData(scoped.queryKey));

  return useQuery({
    ...(cached ? scoped : createQueryOptions(listSilences, {}, { transport })),
    ...cacheOptions,
    select: (response) => findSilence(response) ?? null,
  });
}
