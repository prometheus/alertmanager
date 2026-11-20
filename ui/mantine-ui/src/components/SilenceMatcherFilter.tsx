import { type SubmitEvent, useId } from 'react';

import {
  Button,
  CloseButton,
  Group,
  NativeSelect,
  Paper,
  Pill,
  Popover,
  SegmentedControl,
  Stack,
  Text,
  TextInput,
} from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';

import {
  appendMatcher,
  formatMatcher,
  type MatcherDefinition,
  type MatcherMode,
  matcherOperators,
  moveMatcher,
} from '@/data/silenceFilters';
import type { MatcherType } from '@/gen/common/v3alpha/common_pb';

import {
  type ResolveSilenceTagDrop,
  type SilenceTagDrag,
  useSilenceTagDrop,
} from './SilenceTagDrag';
import { SilenceFilterPill } from './SilenceTags';

type Props = {
  sets: MatcherDefinition[][];
  mode: MatcherMode;
  onChange: (sets: MatcherDefinition[][]) => void;
  onModeChange: (mode: MatcherMode) => void;
  inDrawer: boolean;
};

const setOperators = { any: 'OR', all: 'AND' } as const;

function MatcherFilterHeading({
  headingID,
  mode,
  onModeChange,
  inDrawer,
}: Pick<Props, 'mode' | 'onModeChange' | 'inDrawer'> & { headingID: string }) {
  const descriptionID = useId();
  return (
    <>
      <Group gap="xs" wrap="nowrap">
        <Text component="h2" id={headingID} size="sm" fw={700}>
          Matcher sets
        </Text>
        <SegmentedControl
          size="xs"
          aria-label="Matcher set filter mode"
          aria-describedby={descriptionID}
          value={mode}
          onChange={onModeChange}
          data={[
            { value: 'any', label: 'Any' },
            { value: 'all', label: 'All' },
          ]}
        />
      </Group>
      <Text
        id={descriptionID}
        size="xs"
        c="dimmed"
        flex={inDrawer ? '0 0 100%' : 1}
        style={inDrawer ? { order: 1 } : undefined}
      >
        {mode === 'all'
          ? 'Each filter set must match a different stored set.'
          : 'At least one filter set must match a stored set.'}{' '}
        All matchers in a set must match exactly. Empty sets are ignored.
      </Text>
    </>
  );
}

function AddMatcher({
  setNumber,
  onAdd,
  withinPortal,
}: {
  setNumber: number;
  onAdd: (matcher: MatcherDefinition) => void;
  withinPortal: boolean;
}) {
  const [opened, { open, close }] = useDisclosure();

  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const name = String(form.get('name'));
    const type = Number(form.get('operator')) as MatcherType;
    const value = String(form.get('value'));
    onAdd({ name, type, value });
    close();
  };

  return (
    <Popover
      opened={opened}
      onChange={(next) => (next ? open() : close())}
      position="bottom-start"
      trapFocus
      returnFocus
      withinPortal={withinPortal}
      width="min(18.75rem, calc(100vw - 2rem))"
    >
      <Popover.Target>
        <Button
          variant="subtle"
          size="compact-xs"
          aria-label={`Add matcher to set ${setNumber}`}
          onClick={opened ? close : open}
        >
          + Matcher
        </Button>
      </Popover.Target>
      <Popover.Dropdown>
        <form onSubmit={submit}>
          <Stack gap="sm">
            <TextInput data-autofocus label="Matcher name" name="name" required />
            <NativeSelect
              label="Operator"
              name="operator"
              data={Object.entries(matcherOperators).map(([value, label]) => ({ value, label }))}
            />
            <TextInput label="Matcher value" name="value" />
            <Button type="submit">Apply matcher</Button>
          </Stack>
        </form>
      </Popover.Dropdown>
    </Popover>
  );
}

function MatcherFilterSet({
  matchers,
  index,
  operator,
  onAdd,
  resolveDrop,
  onRemoveMatcher,
  onRemoveSet,
  withinPortal,
}: {
  matchers: MatcherDefinition[];
  index: number;
  operator?: 'AND' | 'OR';
  onAdd: (matcher: MatcherDefinition) => void;
  resolveDrop: ResolveSilenceTagDrop;
  onRemoveMatcher: (index: number) => void;
  onRemoveSet: () => void;
  withinPortal: boolean;
}) {
  const number = index + 1;
  const removeLabel = `Remove matcher set ${number}`;
  const dropProps = useSilenceTagDrop(resolveDrop);
  return (
    <Group gap="xs" wrap="nowrap" miw={0}>
      {operator && matchers.length > 0 && (
        <Text size="xs" fw={600}>
          {operator}
        </Text>
      )}
      <Paper {...dropProps} withBorder px="xs" py={4} miw={0}>
        <Group gap={4} role="group" aria-label={`Matcher filter set ${number}`}>
          <Text size="xs" fw={600}>
            Set {number} (AND)
          </Text>
          {matchers.length ? (
            <Pill.Group maw="100%">
              {matchers.map((matcher, matcherIndex) => {
                const label = formatMatcher(matcher);
                return (
                  <SilenceFilterPill
                    key={matcherIndex}
                    label={label}
                    removeLabel={`Remove matcher ${label}`}
                    onRemove={() => onRemoveMatcher(matcherIndex)}
                    drag={{ kind: 'matcher', matcher, source: { setIndex: index, matcherIndex } }}
                  />
                );
              })}
            </Pill.Group>
          ) : (
            <Text size="xs" c="dimmed">
              No matchers
            </Text>
          )}
          <Group gap={4} wrap="nowrap">
            <AddMatcher setNumber={number} onAdd={onAdd} withinPortal={withinPortal} />
            <CloseButton
              size="xs"
              aria-label={removeLabel}
              title={removeLabel}
              onClick={onRemoveSet}
            />
          </Group>
        </Group>
      </Paper>
    </Group>
  );
}

export function SilenceMatcherFilter({ sets, mode, onChange, onModeChange, inDrawer }: Props) {
  const headingID = useId();
  const firstActiveSet = sets.findIndex((set) => set.length > 0);
  const add = (matcher: MatcherDefinition, index: number) => {
    const next = appendMatcher(sets, matcher, index);
    if (next !== sets) {
      onChange(next);
    }
  };
  const removeSet = (setIndex: number) => onChange(sets.filter((_, index) => index !== setIndex));
  const removeMatcher = (setIndex: number, matchIndex: number) => {
    const remaining = sets[setIndex].filter((_, index) => index !== matchIndex);
    if (remaining.length) {
      onChange(sets.map((set, index) => (index === setIndex ? remaining : set)));
    } else {
      removeSet(setIndex);
    }
  };
  const dropMatcher = (tag: SilenceTagDrag, index: number) => {
    if (tag.kind !== 'matcher') {
      return;
    }
    const { matcher, source } = tag;
    const next = source
      ? moveMatcher(sets, matcher, source, index)
      : appendMatcher(sets, matcher, index);
    if (next !== sets) {
      return () => onChange(next);
    }
  };
  const newSetDrop = useSilenceTagDrop((tag) => dropMatcher(tag, sets.length));

  return (
    <Stack component="section" aria-labelledby={headingID} gap="xs">
      <Group gap="xs" justify="space-between">
        <MatcherFilterHeading
          headingID={headingID}
          mode={mode}
          onModeChange={onModeChange}
          inDrawer={inDrawer}
        />
        <Button {...newSetDrop} size="xs" variant="default" onClick={() => onChange([...sets, []])}>
          Add matcher set
        </Button>
      </Group>
      {sets.length > 0 && (
        <Group gap="xs" role="group" aria-label="Matcher filters">
          {sets.map((set, setIndex) => (
            <MatcherFilterSet
              key={setIndex}
              matchers={set}
              index={setIndex}
              operator={setIndex > firstActiveSet ? setOperators[mode] : undefined}
              onAdd={(matcher) => add(matcher, setIndex)}
              resolveDrop={(tag) => dropMatcher(tag, setIndex)}
              onRemoveMatcher={(matchIndex) => removeMatcher(setIndex, matchIndex)}
              onRemoveSet={() => removeSet(setIndex)}
              withinPortal={!inDrawer}
            />
          ))}
        </Group>
      )}
    </Stack>
  );
}
