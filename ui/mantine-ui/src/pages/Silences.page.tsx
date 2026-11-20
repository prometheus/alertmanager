import { type ReactNode, useId } from 'react';

import {
  Alert,
  Box,
  Button,
  Chip,
  Container,
  Divider,
  Drawer,
  EmptyState,
  Fieldset,
  Group,
  MultiSelect,
  NativeSelect,
  Pagination,
  SegmentedControl,
  Skeleton,
  Stack,
  Text,
  TextInput,
  VisuallyHidden,
} from '@mantine/core';
import { useDisclosure, useMediaQuery, useResizeObserver } from '@mantine/hooks';
import { useLocation, useSearchParams } from 'react-router-dom';

import { SilenceMatcherFilter } from '@/components/SilenceMatcherFilter';
import { SilencesTable } from '@/components/SilencesTable';
import { SilenceTagDragProvider, useSilenceTagDrop } from '@/components/SilenceTagDrag';
import { SilenceFilterPill } from '@/components/SilenceTags';
import {
  annotationOptions,
  appendMatcher,
  encodeAnnotation,
  encodeMatcherSet,
  filterSilences,
  filterSilencesByMatcherSets,
  type MatcherDefinition,
  readSilenceFilters,
  resetSilenceParams,
  type SilenceFilters,
  type SilenceSort,
  type SilenceTagFilters,
  type SortField,
  silenceListInput,
  silenceStates,
  silenceTagFilters,
  sortColumns,
  withSilenceParam,
} from '@/data/silenceFilters';
import { useSilences } from '@/data/silences';
import type { Silence } from '@/gen/silence/v3alpha/silence_pb';

import classes from './Silences.page.module.css';

const sortOptions = [
  { value: '', label: 'Default order' },
  ...sortColumns.flatMap(({ field, label }) => [
    {
      value: `${field}:asc`,
      label: `${label} (${field === 'createdBy' ? 'A to Z' : 'earliest first'})`,
    },
    {
      value: `${field}:desc`,
      label: `${label} (${field === 'createdBy' ? 'Z to A' : 'latest first'})`,
    },
  ]),
];

type ViewProps = {
  filters: SilenceFilters;
  silences: Silence[];
  update: (key: string, values: string[], replace?: boolean) => void;
};

type FilterActions = {
  onAnnotations: SilenceTagFilters['onAnnotations'];
  onMatcherSets: (sets: MatcherDefinition[][]) => void;
};

type FilterPanelProps = ViewProps &
  FilterActions & {
    inDrawer: boolean;
    sortInDrawer: boolean;
    resetAction?: ReactNode;
  };

function SortSilences({
  sort,
  update,
  inDrawer,
}: {
  sort?: SilenceSort;
  update: ViewProps['update'];
  inDrawer?: boolean;
}) {
  const label = 'Sort silences';
  return (
    <NativeSelect
      size={inDrawer ? 'sm' : 'xs'}
      label={inDrawer ? label : undefined}
      aria-label={inDrawer ? undefined : label}
      data={sortOptions}
      value={sort ? `${sort.field}:${sort.direction}` : ''}
      onChange={({ currentTarget: { value } }) => update('sort', value ? [value] : [])}
    />
  );
}

function AnnotationFilter({
  filters,
  silences,
  update,
  onAnnotations,
  inDrawer,
}: ViewProps & Pick<FilterActions, 'onAnnotations'> & { inDrawer: boolean }) {
  const labelID = useId();
  const inputID = useId();
  const dropProps = useSilenceTagDrop((tag) =>
    tag.kind === 'annotation' ? () => onAnnotations([tag.annotation]) : undefined
  );
  return (
    <Stack
      component="section"
      aria-labelledby={labelID}
      className={classes.annotationSection}
      gap={4}
    >
      <Group gap="xs" wrap="nowrap">
        <Text component="h2" id={labelID} size="sm" fw={700}>
          <label htmlFor={inputID}>Annotations</label>
        </Text>
        <SegmentedControl
          size="xs"
          aria-label="Annotation match mode"
          value={filters.annotationMode}
          onChange={(mode) => update('annotation-mode', mode === 'or' ? [mode] : [])}
          data={[
            { value: 'and', label: 'AND' },
            { value: 'or', label: 'OR' },
          ]}
        />
      </Group>
      <MultiSelect
        id={inputID}
        mt="auto"
        description="AND matches every pair. OR matches any pair."
        wrapperProps={dropProps}
        placeholder="Find a key or value"
        data={annotationOptions(silences, filters.annotations)}
        value={filters.annotations.map(encodeAnnotation)}
        onChange={(values) => update('annotation', values)}
        renderPill={({ option, onRemove, disabled }) => (
          <SilenceFilterPill
            label={option.label}
            removeLabel={`Remove annotation ${option.label}`}
            onRemove={onRemove}
            disabled={disabled}
          />
        )}
        searchable
        limit={50}
        clearable
        clearButtonProps={{ 'aria-label': 'Clear annotation filters' }}
        comboboxProps={{ withinPortal: !inDrawer }}
        nothingFoundMessage="No annotations found"
      />
    </Stack>
  );
}

function SearchAndStateFilters({ filters, update }: Pick<ViewProps, 'filters' | 'update'>) {
  const stateLabelID = useId();
  const searchID = useId();
  const changeStates = (selected: string[]) => {
    const states =
      filters.states.length && selected.includes('all')
        ? []
        : selected.filter((state) => state !== 'all');
    update('state', states.length ? states : ['all']);
  };

  return (
    <Stack gap={4}>
      <Group className={classes.stateFilter} gap={4} role="group" aria-labelledby={stateLabelID}>
        <Text component="span" id={stateLabelID} size="sm" fw={700}>
          State
        </Text>
        <Chip.Group
          multiple
          value={filters.states.length ? filters.states : ['all']}
          onChange={changeStates}
        >
          <Chip value="all">All</Chip>
          {silenceStates.map(({ value, label, color }) => (
            <Chip key={value} value={value} color={color}>
              {label}
            </Chip>
          ))}
        </Chip.Group>
      </Group>
      <Group gap="xs" wrap="nowrap" mt="auto">
        <Text component="label" htmlFor={searchID} size="sm" fw={700}>
          Text Search
        </Text>
        <TextInput
          id={searchID}
          flex={1}
          miw={0}
          placeholder="ID, creator, or comment"
          value={filters.search}
          onChange={(event) => {
            const text = event.currentTarget.value;
            update('q', text ? [text] : [], true);
          }}
        />
      </Group>
    </Stack>
  );
}

function SilencesFilters({
  filters,
  silences,
  update,
  onAnnotations,
  onMatcherSets,
  inDrawer,
  sortInDrawer,
  resetAction,
}: FilterPanelProps) {
  return (
    <Stack gap="xs">
      <Group gap="md" align="flex-start" wrap="nowrap">
        <Box className={classes.filterSections} mod={{ drawer: inDrawer }} flex={1} miw={0}>
          <SearchAndStateFilters filters={filters} update={update} />
          <AnnotationFilter
            filters={filters}
            silences={silences}
            update={update}
            onAnnotations={onAnnotations}
            inDrawer={inDrawer}
          />
        </Box>
        {resetAction}
      </Group>
      {sortInDrawer && (
        <>
          <Divider />
          <SortSilences sort={filters.sort} update={update} inDrawer />
        </>
      )}
      <Divider />
      <SilenceMatcherFilter
        sets={filters.matcherSets}
        mode={filters.matcherMode}
        onChange={onMatcherSets}
        onModeChange={(mode) => update('matcher-mode', mode === 'all' ? [mode] : [])}
        inDrawer={inDrawer}
      />
    </Stack>
  );
}

function fittedRows(height: number, rowHeight: number) {
  return rowHeight ? Math.max(1, Math.floor(height / rowHeight)) : 1;
}

function useSilenceTableSpace() {
  const [tableAreaRef, { width: areaWidth, height: tableAreaHeight }] =
    useResizeObserver<HTMLElement>();
  const [headerRef, { height: headerHeight }] = useResizeObserver<HTMLTableSectionElement>();
  const [rowRef, { height: rowHeight }] = useResizeObserver<HTMLTableRowElement>();
  const [expandedRef, { height: expandedHeight }] = useResizeObserver<HTMLTableRowElement>();
  const bodyHeight = tableAreaHeight - headerHeight;
  return {
    tableAreaRef,
    areaWidth,
    headerRef,
    rowRef,
    expandedRef,
    pageSize: fittedRows(bodyHeight, rowHeight),
    expandedCapacity: expandedHeight ? fittedRows(bodyHeight - expandedHeight, rowHeight) : 1,
    expansionLimit: Math.max(0, Math.floor(bodyHeight - rowHeight - 1)),
  };
}

function silencePageRows(
  silences: Silence[],
  page: number,
  pageSize: number,
  expandedID: string | null,
  expandedCapacity: number
) {
  const offset = (page - 1) * pageSize;
  const rows = silences.slice(offset, offset + pageSize);
  const expandedIndex = rows.findIndex(({ id }) => id === expandedID);
  if (expandedIndex < 0) {
    return { rows, offset, expandedID: undefined };
  }

  // Keep the expanded silence visible when its panel uses space from the other rows.
  const first = Math.max(0, expandedIndex + 1 - expandedCapacity);
  return {
    rows: rows.slice(first, first + expandedCapacity),
    offset: offset + first,
    expandedID: rows[expandedIndex].id,
  };
}

function nextSort(sort: SilenceSort | undefined, field: SortField) {
  if (sort?.field === field) {
    return `${field}:${sort.direction === 'asc' ? 'desc' : 'asc'}`;
  }
  return `${field}:${field === 'updatedAt' ? 'desc' : 'asc'}`;
}

function SilencesResults({
  filters,
  silences,
  update,
  search,
  tagFilters,
}: ViewProps & { search: string; tagFilters: SilenceTagFilters }) {
  const {
    tableAreaRef,
    areaWidth,
    headerRef,
    rowRef,
    expandedRef,
    pageSize,
    expandedCapacity,
    expansionLimit,
  } = useSilenceTableSpace();
  const { expandedID, sort } = filters;
  const filtered = filterSilences(silences, filters);
  if (!filtered.length) {
    return (
      <EmptyState
        title="No silences match"
        description="Change the filters or select All to include expired silences."
      />
    );
  }
  const totalPages = Math.ceil(filtered.length / pageSize);
  const page = Math.min(filters.page, totalPages);
  const {
    rows,
    offset,
    expandedID: visibleExpandedID,
  } = silencePageRows(filtered, page, pageSize, expandedID, expandedCapacity);
  const expansion = visibleExpandedID
    ? { id: visibleExpandedID, maxHeight: expansionLimit, ref: expandedRef }
    : undefined;

  return (
    <Stack className={classes.results} gap="xs">
      <section className={classes.tableArea} ref={tableAreaRef} aria-label="Silence results">
        <SilencesTable
          silences={rows}
          search={search}
          sort={sort}
          areaWidth={areaWidth}
          headerRef={headerRef}
          rowRef={rowRef}
          expansion={expansion}
          onToggle={(id) => update('expanded', expandedID === id ? [] : [id], true)}
          onSort={(field) => update('sort', [nextSort(sort, field)])}
          {...tagFilters}
        />
      </section>
      <Group className={classes.footer} justify="space-between" gap="xs">
        <Text size="sm" c="dimmed" aria-live="polite">
          {offset + 1}–{offset + rows.length} of {filtered.length} silences
        </Text>
        <nav className={classes.pagination} aria-label="Silence pages">
          <Pagination
            total={totalPages}
            value={page}
            onChange={(value) => update('page', value === 1 ? [] : [String(value)])}
            size="sm"
            gap={4}
            hideWithOnePage
            getItemProps={(value) => ({ 'aria-label': `Page ${value}` })}
            getControlProps={(control) => ({ 'aria-label': `${control} page` })}
          />
        </nav>
      </Group>
    </Stack>
  );
}

function SilencesControls({
  refresh,
  refreshing,
  resetView,
  resetFilters,
  onAnnotations,
  onMatcherSets,
  ...view
}: ViewProps &
  FilterActions & {
    refresh: () => void;
    refreshing: boolean;
    resetView?: () => void;
    resetFilters?: () => void;
  }) {
  const compact = useMediaQuery('(max-width: 48em), (max-height: 42em)', undefined, {
    getInitialValueInEffect: false,
  });
  const sortInDrawer = useMediaQuery('(max-width: 34em), (max-height: 24em)', undefined, {
    getInitialValueInEffect: false,
  });
  const [filtersOpened, { open: openFilters, close: closeFilters }] = useDisclosure();
  const filtersDrop = useSilenceTagDrop((tag) => {
    if (tag.kind === 'matcher' && tag.source) {
      return;
    }
    return () => {
      if (tag.kind === 'matcher') {
        const { matcherSets } = view.filters;
        onMatcherSets(appendMatcher(matcherSets, tag.matcher, matcherSets.length));
      } else {
        onAnnotations([tag.annotation]);
      }
      openFilters();
    };
  });
  const gap = compact ? 4 : 'xs';
  const buttonPadding = compact ? 6 : undefined;
  const resetFiltersButton = (
    <Button
      size="xs"
      px={buttonPadding}
      variant="default"
      disabled={!resetFilters}
      onClick={resetFilters}
    >
      Reset filters
    </Button>
  );
  const filterControls = (
    <SilencesFilters
      {...view}
      onAnnotations={onAnnotations}
      onMatcherSets={onMatcherSets}
      inDrawer={compact}
      sortInDrawer={sortInDrawer}
      resetAction={compact ? undefined : resetFiltersButton}
    />
  );
  return (
    <>
      {compact ? (
        <Drawer.Root opened={filtersOpened} onClose={closeFilters} position="right">
          <Drawer.Overlay />
          <Drawer.Content>
            <Drawer.Header>
              <Drawer.Title>Silence filters</Drawer.Title>
              <Group gap="xs" wrap="nowrap">
                {resetFiltersButton}
                <Drawer.CloseButton aria-label="Close filters" />
              </Group>
            </Drawer.Header>
            <Drawer.Body>{filterControls}</Drawer.Body>
          </Drawer.Content>
        </Drawer.Root>
      ) : (
        <Fieldset
          legend="Filters"
          className={classes.filters}
          classNames={{ legend: classes.filtersLegend }}
          m={0}
          px="md"
          pt={4}
          pb="sm"
        >
          {filterControls}
        </Fieldset>
      )}
      <Group gap={gap} role="group" aria-label="Silence list controls">
        {compact && (
          <Button
            {...filtersDrop}
            size="xs"
            px={buttonPadding}
            variant="default"
            onClick={openFilters}
          >
            Filters
          </Button>
        )}
        <Text className={classes.timezone} size="sm" c="dimmed">
          Times in UTC
        </Text>
        <Group gap={gap} wrap="nowrap" ml="auto">
          {!sortInDrawer && <SortSilences sort={view.filters.sort} update={view.update} />}
          <Button
            size="xs"
            px={buttonPadding}
            variant="default"
            disabled={!resetView}
            onClick={resetView}
          >
            Reset view
          </Button>
          <Button
            size="xs"
            px={buttonPadding}
            variant="default"
            loading={refreshing}
            onClick={refresh}
          >
            Refresh
          </Button>
          <Button size="xs" px={buttonPadding} disabled>
            New silence
          </Button>
        </Group>
      </Group>
    </>
  );
}

export function SilencesPage() {
  const { search } = useLocation();
  const [params, setParams] = useSearchParams();
  const filters = readSilenceFilters(params);
  const viewResetParams = resetSilenceParams(params, 'view');
  const filterResetParams = resetSilenceParams(params, 'filters');
  const { data, isPending, isError, isFetching, refetch } = useSilences(silenceListInput(filters));
  const silences = filterSilencesByMatcherSets(data?.silences ?? [], filters);
  const update: ViewProps['update'] = (key, values, replace = false) =>
    setParams(withSilenceParam(params, key, values), { replace });
  const tagFilters = silenceTagFilters(params, filters, (next) => {
    if (next !== params) {
      setParams(next);
    }
  });

  return (
    <Container className={classes.page} size={1440} px={0}>
      <VisuallyHidden role="heading" aria-level={1}>
        Silences
      </VisuallyHidden>
      <SilenceTagDragProvider>
        <SilencesControls
          filters={filters}
          silences={silences}
          update={update}
          onAnnotations={tagFilters.onAnnotations}
          onMatcherSets={(sets) => update('matcher', sets.map(encodeMatcherSet))}
          resetView={viewResetParams && (() => setParams(viewResetParams))}
          resetFilters={filterResetParams && (() => setParams(filterResetParams))}
          refresh={() => void refetch()}
          refreshing={isFetching && !isPending}
        />
        {isError && (
          <Alert color="red" title="Could not load silences">
            {data
              ? 'Showing the last successful result. Select Refresh to try again.'
              : 'Select Refresh to try again.'}
          </Alert>
        )}
        {isPending && (
          <Stack gap="xs" role="status" aria-label="Loading silences">
            <Skeleton height={40} />
            <Skeleton height={65} />
            <Skeleton height={65} />
          </Stack>
        )}
        {data && (
          <SilencesResults
            filters={filters}
            silences={silences}
            update={update}
            search={search}
            tagFilters={tagFilters}
          />
        )}
      </SilenceTagDragProvider>
    </Container>
  );
}
