import { type Ref, useId } from 'react';

import {
  AccordionChevron,
  ActionIcon,
  Anchor,
  Group,
  Stack,
  Table,
  Text,
  UnstyledButton,
  VisuallyHidden,
} from '@mantine/core';
import { Link } from 'react-router-dom';

import {
  type Annotation,
  listAnnotations,
  type SilenceSort,
  type SilenceTagActions,
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

type Props = {
  silences: Silence[];
  search: string;
  sort?: SilenceSort;
  areaWidth: number;
  headerRef: Ref<HTMLTableSectionElement>;
  rowRef: Ref<HTMLTableRowElement>;
  expansion?: Expansion;
  onToggle: (id: string) => void;
  onSort: (field: SortField) => void;
  tagActions: SilenceTagActions;
};

type RowDisclosure = {
  'aria-expanded': boolean;
  'aria-controls'?: string;
  onClick: () => void;
};
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
  const expanded = disclosure['aria-expanded'];
  const label = expanded ? `Hide ${kind} details` : `Show ${description}`;
  return (
    <UnstyledButton
      {...disclosure}
      type="button"
      className={classes.more}
      title={label}
      aria-label={label}
    >
      {expanded ? '−' : summary}
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
  addMatcher,
  disclosure,
}: Pick<SilenceTagActions, 'addMatcher'> & { sets: MatcherSet[]; disclosure: RowDisclosure }) {
  const matchers = sets[0]?.matchers ?? [];
  const multiple = sets.length > 1;
  return (
    <TagPreview
      tags={matchers
        .slice(0, multiple ? 1 : 2)
        .map((matcher) => matcherTagProps(matcher, addMatcher))}
      count={multiple ? sets.length : Math.max(0, matchers.length - 2)}
      kind={multiple ? 'matcher set' : 'matcher'}
      disclosure={disclosure}
    />
  );
}

const tableSortColumns = sortColumns.map((column) => ({
  ...column,
  className: column.field === 'createdBy' ? classes.creator : classes.timestamp,
}));

function visibleColumns(width: number) {
  const state = !width || width >= 480;
  const sort = tableSortColumns.filter(({ minWidth }) => !width || width >= minWidth);
  return { state, sort, count: 4 + Number(state) + sort.length };
}

function ExpandedSilenceRow({
  panelId,
  expansion,
  columnCount,
  silence,
  annotations,
  tagActions: { addMatcher, addMatcherSet, addAnnotations },
}: {
  panelId: string;
  expansion: Expansion;
  columnCount: number;
  silence: Silence;
  annotations: Annotation[];
  tagActions: SilenceTagActions;
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
                addMatcher={addMatcher}
                addMatcherSet={addMatcherSet}
              />
            </div>
            <SilenceAnnotations annotations={annotations} addAnnotations={addAnnotations} compact />
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
          <Anchor component={Link} to={to} truncate="end" size="sm" title={name}>
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
  tagActions,
}: Pick<Props, 'search' | 'onToggle' | 'expansion' | 'tagActions'> & {
  rowRef?: Props['rowRef'];
  silence: Silence;
  columns: ReturnType<typeof visibleColumns>;
}) {
  const panelId = useId();
  const disclosure = {
    'aria-expanded': Boolean(expansion),
    'aria-controls': expansion ? panelId : undefined,
    onClick: () => onToggle(silence.id),
  };
  const toggleLabel = expansion ? 'Hide tags' : 'Show tags';
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
            addMatcher={tagActions.addMatcher}
            disclosure={disclosure}
          />
        </Table.Td>
        <Table.Td>
          <TagPreview
            tags={annotations
              .slice(0, 2)
              .map((annotation) => annotationTagProps(annotation, tagActions.addAnnotations))}
            count={Math.max(0, annotations.length - 2)}
            kind="annotation"
            disclosure={disclosure}
          />
        </Table.Td>
        {columns.sort.map(({ field, className }) => (
          <Table.Td key={field} className={className}>
            {field === 'createdBy' ? (
              <Text size="sm" truncate title={silence.createdBy}>
                {silence.createdBy || '—'}
              </Text>
            ) : (
              <SilenceTime value={silence[field]} compact />
            )}
          </Table.Td>
        ))}
        <Table.Td className={classes.tagToggleColumn}>
          <ActionIcon
            {...disclosure}
            type="button"
            variant="subtle"
            size={24}
            aria-label={toggleLabel}
            title={toggleLabel}
          >
            <AccordionChevron aria-hidden="true" focusable="false" />
          </ActionIcon>
        </Table.Td>
      </Table.Tr>
      {expansion && (
        <ExpandedSilenceRow
          panelId={panelId}
          expansion={expansion}
          columnCount={columns.count}
          silence={silence}
          annotations={annotations}
          tagActions={tagActions}
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
  tagActions,
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
          {columns.sort.map(({ field, label, className }) => {
            const direction = sort?.field === field ? sort.direction : undefined;
            return (
              <Table.Th
                key={field}
                scope="col"
                className={className}
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
          <Table.Th scope="col" className={classes.tagToggleColumn}>
            <VisuallyHidden>Tag details</VisuallyHidden>
          </Table.Th>
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
            tagActions={tagActions}
          />
        ))}
      </Table.Tbody>
    </Table>
  );
}
