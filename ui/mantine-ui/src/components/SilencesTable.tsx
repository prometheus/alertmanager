import { type Ref, useId } from 'react';

import { Anchor, Group, Stack, Table, Text, UnstyledButton } from '@mantine/core';
import { Link } from 'react-router-dom';

import {
  type Annotation,
  listAnnotations,
  type SilenceSort,
  type SilenceTagFilters,
  type SortField,
  sortColumns,
} from '@/data/silenceFilters';
import type { MatcherSet } from '@/gen/common/v3alpha/common_pb';
import type { Silence } from '@/gen/silence/v3alpha/silence_pb';

import { SilenceStateBadge, SilenceTime } from './SilenceFields';
import classes from './SilencesTable.module.css';
import {
  annotationTagProps,
  matcherTagProps,
  SilenceAnnotations,
  SilenceMatcherSets,
  SilenceTag,
  type SilenceTagProps,
} from './SilenceTags';

type Expansion = { id: string; maxHeight: number; ref: Ref<HTMLTableRowElement> };

type Props = SilenceTagFilters & {
  silences: Silence[];
  search: string;
  sort?: SilenceSort;
  areaWidth: number;
  headerRef: Ref<HTMLTableSectionElement>;
  rowRef: Ref<HTMLTableRowElement>;
  expansion?: Expansion;
  onToggle: (id: string) => void;
  onSort: (field: SortField) => void;
};

type RowDisclosure = { panelId: string; expanded: boolean; toggle: () => void };
type TagKind = 'matcher' | 'matcher set' | 'annotation';

function MoreTags({
  count,
  kind,
  disclosure,
}: {
  count: number;
  kind: TagKind;
  disclosure: RowDisclosure;
}) {
  const sets = kind === 'matcher set';
  const summary = sets ? `${count} sets` : `+${count}`;
  const description = `${count}${sets ? '' : ' more'} ${kind}${count === 1 ? '' : 's'}`;
  const label = disclosure.expanded ? `Hide ${kind} details` : `Show ${description}`;
  return (
    <UnstyledButton
      type="button"
      className={classes.more}
      title={label}
      aria-label={label}
      aria-expanded={disclosure.expanded}
      aria-controls={disclosure.expanded ? disclosure.panelId : undefined}
      onClick={disclosure.toggle}
    >
      {disclosure.expanded ? '−' : summary}
    </UnstyledButton>
  );
}

function TagPreview({
  tags: [first, second],
  count,
  kind,
  disclosure,
}: {
  tags: SilenceTagProps[];
  count: number;
  kind: TagKind;
  disclosure: RowDisclosure;
}) {
  if (!first && !count) {
    return <Text c="dimmed">—</Text>;
  }
  return (
    <Stack gap={4} align="flex-start">
      {first && <SilenceTag {...first} />}
      {(second || count > 0) && (
        <Group gap={4} wrap="nowrap" w="100%">
          {second && <SilenceTag {...second} />}
          {count > 0 && <MoreTags count={count} kind={kind} disclosure={disclosure} />}
        </Group>
      )}
    </Stack>
  );
}

function MatcherPreview({
  sets,
  onMatcher,
  disclosure,
}: Pick<SilenceTagFilters, 'onMatcher'> & { sets: MatcherSet[]; disclosure: RowDisclosure }) {
  const matchers = sets[0]?.matchers ?? [];
  const multiple = sets.length > 1;
  return (
    <TagPreview
      tags={matchers
        .slice(0, multiple ? 1 : 2)
        .map((matcher) => matcherTagProps(matcher, onMatcher))}
      count={multiple ? sets.length : Math.max(0, matchers.length - 2)}
      kind={multiple ? 'matcher set' : 'matcher'}
      disclosure={disclosure}
    />
  );
}

const sortColumnClasses = {
  createdBy: classes.creator,
  startsAt: classes.timestamp,
  endsAt: classes.timestamp,
  updatedAt: classes.timestamp,
};

function visibleColumns(width: number) {
  const state = !width || width >= 480;
  const sort =
    !width || width >= 1120
      ? sortColumns
      : sortColumns.filter(
          ({ field }) =>
            (field === 'endsAt' && width >= 704) || (field === 'updatedAt' && width >= 864)
        );
  return { state, sort, count: (state ? 4 : 3) + sort.length };
}

function ExpandedSilenceRow({
  panelId,
  expansion,
  columnCount,
  silence,
  annotations,
  onMatcher,
  onMatcherSet,
  onAnnotations,
}: SilenceTagFilters & {
  panelId: string;
  expansion: Expansion;
  columnCount: number;
  silence: Silence;
  annotations: Annotation[];
}) {
  return (
    <Table.Tr ref={expansion.ref} data-expanded-row>
      <Table.Td colSpan={columnCount} className={classes.expandedCell}>
        <section
          id={panelId}
          aria-label="Expanded silence tags"
          className={classes.expandedTags}
          style={{ maxHeight: expansion.maxHeight }}
        >
          <div className={classes.tagGroups}>
            <div>
              <Text size="xs" fw={600} mb={4}>
                Matcher sets
              </Text>
              <SilenceMatcherSets
                sets={silence.matcherSets}
                onMatcher={onMatcher}
                onMatcherSet={onMatcherSet}
              />
            </div>
            <SilenceAnnotations annotations={annotations} onAnnotations={onAnnotations} compact />
          </div>
        </section>
      </Table.Td>
    </Table.Tr>
  );
}

function SilenceSummaryCell({
  silence,
  search,
  inlineState,
}: {
  silence: Silence;
  search: string;
  inlineState: boolean;
}) {
  const to = { pathname: `/silences/${encodeURIComponent(silence.id)}`, search };
  const name = silence.comment.trim();
  return (
    <Table.Td>
      <Group gap={4} wrap="nowrap">
        {name && (
          <Anchor component={Link} to={to} className={classes.truncate} size="sm" title={name}>
            {name}
          </Anchor>
        )}
        {inlineState && (
          <span className={classes.inlineState}>
            <SilenceStateBadge state={silence.status?.state} />
          </span>
        )}
      </Group>
      <Anchor component={Link} to={to} className={classes.silenceID} size="xs" ff="monospace">
        {silence.id}
      </Anchor>
    </Table.Td>
  );
}

function SilenceRow({
  silence,
  search,
  rowRef,
  expansion,
  columns,
  onToggle,
  onMatcher,
  onMatcherSet,
  onAnnotations,
}: SilenceTagFilters &
  Pick<Props, 'search' | 'onToggle' | 'expansion'> & {
    rowRef?: Props['rowRef'];
    silence: Silence;
    columns: ReturnType<typeof visibleColumns>;
  }) {
  const panelId = useId();
  const disclosure = { panelId, expanded: Boolean(expansion), toggle: () => onToggle(silence.id) };
  const annotations = listAnnotations(silence);
  const state = silence.status?.state;

  return (
    <>
      <Table.Tr ref={rowRef}>
        {columns.state && (
          <Table.Td className={classes.stateColumn}>
            <SilenceStateBadge state={state} />
          </Table.Td>
        )}
        <SilenceSummaryCell silence={silence} search={search} inlineState={!columns.state} />
        <Table.Td>
          <MatcherPreview
            sets={silence.matcherSets}
            onMatcher={onMatcher}
            disclosure={disclosure}
          />
        </Table.Td>
        <Table.Td>
          <TagPreview
            tags={annotations
              .slice(0, 2)
              .map((annotation) => annotationTagProps(annotation, onAnnotations))}
            count={Math.max(0, annotations.length - 2)}
            kind="annotation"
            disclosure={disclosure}
          />
        </Table.Td>
        {columns.sort.map(({ field }) => (
          <Table.Td key={field} className={sortColumnClasses[field]}>
            {field === 'createdBy' ? (
              <Text size="sm" truncate title={silence.createdBy}>
                {silence.createdBy || '—'}
              </Text>
            ) : (
              <SilenceTime value={silence[field]} compact />
            )}
          </Table.Td>
        ))}
      </Table.Tr>
      {expansion && (
        <ExpandedSilenceRow
          panelId={panelId}
          expansion={expansion}
          columnCount={columns.count}
          silence={silence}
          annotations={annotations}
          onMatcher={onMatcher}
          onMatcherSet={onMatcherSet}
          onAnnotations={onAnnotations}
        />
      )}
    </>
  );
}

export function SilencesTable({
  silences,
  search,
  sort,
  areaWidth,
  headerRef,
  rowRef,
  expansion,
  onToggle,
  onSort,
  onMatcher,
  onMatcherSet,
  onAnnotations,
}: Props) {
  const columns = visibleColumns(areaWidth);
  return (
    <Table
      className={classes.table}
      aria-label="Silences"
      layout="fixed"
      horizontalSpacing={8}
      verticalSpacing={6}
      highlightOnHover
      tabularNums
      withRowBorders={false}
    >
      <Table.Thead ref={headerRef}>
        <Table.Tr>
          {columns.state && (
            <Table.Th scope="col" className={classes.stateColumn}>
              State
            </Table.Th>
          )}
          <Table.Th scope="col" className={classes.silenceColumn}>
            Silence
          </Table.Th>
          <Table.Th scope="col">Matchers</Table.Th>
          <Table.Th scope="col">Annotations</Table.Th>
          {columns.sort.map(({ field, label }) => {
            const direction = sort?.field === field ? sort.direction : undefined;
            return (
              <Table.Th
                key={field}
                scope="col"
                className={sortColumnClasses[field]}
                aria-sort={direction && (direction === 'asc' ? 'ascending' : 'descending')}
              >
                <UnstyledButton type="button" fz="sm" w="100%" onClick={() => onSort(field)}>
                  <Group gap={4} wrap="nowrap">
                    {label}
                    <span aria-hidden="true">
                      {direction === 'asc' ? '↑' : direction === 'desc' ? '↓' : '↕'}
                    </span>
                  </Group>
                </UnstyledButton>
              </Table.Th>
            );
          })}
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {silences.map((silence, index) => (
          <SilenceRow
            key={silence.id}
            silence={silence}
            search={search}
            rowRef={index === 0 ? rowRef : undefined}
            expansion={expansion?.id === silence.id ? expansion : undefined}
            columns={columns}
            onToggle={onToggle}
            onMatcher={onMatcher}
            onMatcherSet={onMatcherSet}
            onAnnotations={onAnnotations}
          />
        ))}
      </Table.Tbody>
    </Table>
  );
}
