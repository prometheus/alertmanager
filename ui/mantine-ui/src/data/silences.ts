import type { MessageInitShape } from '@bufbuild/protobuf';
import { createConnectQueryKey, useQuery, useTransport } from '@connectrpc/connect-query';
import { useQueryClient } from '@tanstack/react-query';

import { type ListSilencesResponse, SilenceService } from '@/gen/silence/v3alpha/silence_pb';

const listSilences = SilenceService.method.listSilences;
const cacheOptions = { staleTime: 30_000, gcTime: 60_000, retry: false };

export type SilenceListInput = MessageInitShape<typeof listSilences.input>;

export function useSilences(input: SilenceListInput) {
  return useQuery(listSilences, input, cacheOptions);
}

export function useSilence(id: string, listInput: SilenceListInput) {
  const transport = useTransport();
  const client = useQueryClient();
  const queryKey = createConnectQueryKey({
    schema: listSilences,
    input: listInput,
    transport,
    cardinality: 'finite',
  });
  const findSilence = (response?: ListSilencesResponse) =>
    response?.silences.find((silence) => silence.id === id);
  const cached = findSilence(client.getQueryData(queryKey));

  return useQuery(listSilences, cached ? listInput : {}, {
    ...cacheOptions,
    select: (response) => findSilence(response) ?? null,
  });
}
