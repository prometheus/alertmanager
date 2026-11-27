import { useEffect } from 'react';

import {
  Alert,
  Anchor,
  Button,
  Code,
  CopyButton,
  DataList,
  EmptyState,
  Group,
  Skeleton,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import InfoPageCard from '@/components/InfoPageCard';
import InfoPageStack from '@/components/InfoPageStack';
import { SilenceStateBadge, SilenceTime } from '@/components/SilenceFields';
import { SilenceAnnotations, SilenceMatcherSets } from '@/components/SilenceTags';
import {
  listAnnotations,
  readSilenceFilters,
  type SilenceTagActions,
  silenceActions,
  silenceListInput,
} from '@/data/silenceFilters';
import { useSilence } from '@/data/silences';
import { type Silence, SilenceState } from '@/gen/silence/v3alpha/silence_pb';

import classes from './Silence.page.module.css';

function DetailField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <DataList.Item>
      <DataList.ItemLabel className={classes.label}>{label}</DataList.ItemLabel>
      <DataList.ItemValue className={classes.value}>{children}</DataList.ItemValue>
    </DataList.Item>
  );
}

function SilenceDetails({
  silence,
  tagActions: { addMatcher, addMatcherSet, addAnnotations },
}: {
  silence: Silence;
  tagActions: SilenceTagActions;
}) {
  return (
    <>
      <InfoPageCard title="Details">
        <DataList withDivider>
          <DetailField label="ID">
            <Group gap="xs">
              <Code className={classes.value}>{silence.id}</Code>
              <CopyButton value={silence.id}>
                {({ copied, copy }) => (
                  <Button size="compact-xs" variant="default" onClick={copy}>
                    {copied ? 'Copied' : 'Copy ID'}
                  </Button>
                )}
              </CopyButton>
            </Group>
          </DetailField>
          <DetailField label="Created by">{silence.createdBy || '—'}</DetailField>
          <DetailField label="Comment">{silence.comment || '—'}</DetailField>
          <DetailField label="Starts">
            <SilenceTime value={silence.startsAt} />
          </DetailField>
          <DetailField label="Ends">
            <SilenceTime value={silence.endsAt} />
          </DetailField>
          <DetailField label="Last updated">
            <SilenceTime value={silence.updatedAt} />
          </DetailField>
        </DataList>
      </InfoPageCard>
      <section id="matchers" className={classes.section} aria-label="Matcher sets">
        <InfoPageCard title="Matcher sets">
          <Stack gap="xs">
            <Text size="sm" c="dimmed">
              All matchers in a set must match. Any set can match.
            </Text>
            <SilenceMatcherSets
              sets={silence.matcherSets}
              addMatcher={addMatcher}
              addMatcherSet={addMatcherSet}
            />
          </Stack>
        </InfoPageCard>
      </section>
      {silence.receiverMatcherSets.length > 0 && (
        <InfoPageCard title="Receiver matcher sets">
          <Stack gap="xs">
            <Text size="sm" c="dimmed">
              Alertmanager does not use these matchers for muting.
            </Text>
            <SilenceMatcherSets sets={silence.receiverMatcherSets} />
          </Stack>
        </InfoPageCard>
      )}
      <section id="annotations" className={classes.section} aria-label="Annotations">
        <InfoPageCard>
          <SilenceAnnotations
            annotations={listAnnotations(silence)}
            addAnnotations={addAnnotations}
          />
        </InfoPageCard>
      </section>
    </>
  );
}

function SilenceActions({ state }: { state?: SilenceState }) {
  return (
    <Group gap="xs">
      {state === SilenceState.EXPIRED ? (
        <Button disabled>Recreate</Button>
      ) : (
        <>
          <Button disabled>Edit</Button>
          <Button disabled>Expire</Button>
        </>
      )}
      <Text size="sm" c="dimmed">
        Silence changes are not available in this UI.
      </Text>
    </Group>
  );
}

function SilenceHeading({ search, silence }: { search: string; silence?: Silence | null }) {
  const state = silence?.status?.state;
  return (
    <Stack gap="xs" align="flex-start">
      <Anchor component={Link} to={{ pathname: '/silences', search }}>
        Back to silences
      </Anchor>
      <Group>
        <Title order={1}>Silence</Title>
        {silence && <SilenceStateBadge state={state} />}
      </Group>
      {silence && <SilenceActions state={state} />}
    </Stack>
  );
}

export function SilencePage() {
  const { id = '' } = useParams();
  const { hash, search } = useLocation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const filters = readSilenceFilters(params);
  const { data: silence, isPending, isError, refetch } = useSilence(id, silenceListInput(filters));
  const loaded = Boolean(silence);
  const { tags } = silenceActions(params, filters, (next) =>
    navigate({ pathname: '/silences', search: next.toString() })
  );

  useEffect(() => {
    if (loaded && hash) {
      document.getElementById(hash.slice(1))?.scrollIntoView();
    }
  }, [hash, loaded]);

  return (
    <InfoPageStack>
      <SilenceHeading search={search} silence={silence} />
      {isError && (
        <Alert color="red" title="Could not load silence">
          <Group gap="xs">
            {silence && <Text size="sm">Showing the last successful result.</Text>}
            <Button size="compact-sm" variant="default" onClick={() => void refetch()}>
              Try again
            </Button>
          </Group>
        </Alert>
      )}
      {isPending && <Skeleton height={220} role="status" aria-label="Loading silence" />}
      {silence === null && !isError && (
        <EmptyState title="Silence not found" description="The silence may have been removed." />
      )}
      {silence && <SilenceDetails silence={silence} tagActions={tags} />}
    </InfoPageStack>
  );
}
