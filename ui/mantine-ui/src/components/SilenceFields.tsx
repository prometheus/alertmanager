import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';
import { Badge, Text } from '@mantine/core';

import { silenceStates } from '@/data/silenceFilters';
import type { SilenceState } from '@/gen/silence/v3alpha/silence_pb';

export function SilenceStateBadge({ state }: { state?: SilenceState }) {
  const info = silenceStates.find((item) => item.state === state);
  return (
    <Badge variant="light" color={info?.color ?? 'gray'} tt="none" px={8}>
      {info?.label ?? 'Unknown'}
    </Badge>
  );
}

export function SilenceTime({ value, compact }: { value?: Timestamp; compact?: boolean }) {
  if (!value) {
    return (
      <Text c="dimmed" size="sm">
        Unknown
      </Text>
    );
  }
  const iso = timestampDate(value).toISOString();
  return (
    <time dateTime={iso} title={iso}>
      {compact
        ? `${iso.slice(0, 10)} ${iso.slice(11, 16)}`
        : iso.replace('T', ' ').replace('Z', ' UTC')}
    </time>
  );
}
