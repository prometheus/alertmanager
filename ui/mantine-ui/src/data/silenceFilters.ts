import type { Timestamp } from '@bufbuild/protobuf/wkt';

import { type Matcher, MatcherType } from '@/gen/common/v3alpha/common_pb';
import { type Silence, SilenceState } from '@/gen/silence/v3alpha/silence_pb';

import type { SilenceListInput } from './silences';

export const silenceStates = [
  { value: 'active', label: 'Active', state: SilenceState.ACTIVE, color: 'teal' },
  { value: 'pending', label: 'Pending', state: SilenceState.PENDING, color: 'yellow' },
  { value: 'expired', label: 'Expired', state: SilenceState.EXPIRED, color: 'gray' },
];
const defaultStates = ['active', 'pending'];

export const matcherOperators: Readonly<Partial<Record<MatcherType, string>>> = {
  [MatcherType.EQUAL]: '=',
  [MatcherType.NOT_EQUAL]: '!=',
  [MatcherType.REGEXP]: '=~',
  [MatcherType.NOT_REGEXP]: '!~',
};

export const sortColumns = [
  { field: 'createdBy', label: 'Created by' },
  { field: 'startsAt', label: 'Starts' },
  { field: 'endsAt', label: 'Ends' },
  { field: 'updatedAt', label: 'Last updated' },
] as const;

export type MatcherDefinition = Pick<Matcher, 'name' | 'type' | 'value'>;
export type MatcherPosition = { setIndex: number; matcherIndex: number };
export type MatcherMode = 'any' | 'all';
export type Annotation = [key: string, value: string];
type AnnotationMode = 'and' | 'or';
export type SortField = (typeof sortColumns)[number]['field'];
export type SilenceSort = { field: SortField; direction: 'asc' | 'desc' };

const encodeMatcher = ({ name, type, value }: MatcherDefinition) => [name, type, value];

export const encodeMatcherSet = (set: MatcherDefinition[]) =>
  JSON.stringify(set.map(encodeMatcher));
export const encodeAnnotation = (annotation: Annotation) => JSON.stringify(annotation);

export function formatMatcher({ name, type, value }: MatcherDefinition) {
  const label = /^[a-zA-Z_][a-zA-Z0-9_]*$/.test(name) ? name : JSON.stringify(name);
  return `${label}${matcherOperators[type] ?? '?'}${JSON.stringify(value)}`;
}

export function formatAnnotation([key, value]: Annotation) {
  return `${key}: ${value === '' ? '""' : value}`;
}

function sameMatcher({ name, type, value }: MatcherDefinition, matcher: MatcherDefinition) {
  return name === matcher.name && type === matcher.type && value === matcher.value;
}

function containsMatcher(set: MatcherDefinition[], matcher: MatcherDefinition) {
  return set.some((item) => sameMatcher(item, matcher));
}

function containsMatcherSet(set: MatcherDefinition[], filters: MatcherDefinition[]) {
  return filters.every((matcher) => containsMatcher(set, matcher));
}

export function appendMatcher(
  sets: MatcherDefinition[][],
  matcher: MatcherDefinition,
  index: number
) {
  const set = sets[index];
  if (!set) {
    return [...sets, [matcher]];
  }
  if (containsMatcher(set, matcher)) {
    return sets;
  }
  return sets.map((items, i) => (i === index ? [...items, matcher] : items));
}

export function moveMatcher(
  sets: MatcherDefinition[][],
  matcher: MatcherDefinition,
  { setIndex, matcherIndex }: MatcherPosition,
  targetIndex: number
) {
  const source = sets[setIndex]?.[matcherIndex];
  if (setIndex === targetIndex || !source || !sameMatcher(source, matcher)) {
    return sets;
  }
  const next = appendMatcher(sets, matcher, targetIndex);
  if (next !== sets) {
    // Keep the source as a draft if its last matcher moves.
    next[setIndex] = sets[setIndex].filter((_, index) => index !== matcherIndex);
  }
  return next;
}

function parseArray(value: string): unknown[] | undefined {
  try {
    const parsed: unknown = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

function decodeMatcherSet(value: string): MatcherDefinition[] | undefined {
  const items = parseArray(value);
  if (!items) {
    return;
  }
  const set: MatcherDefinition[] = [];
  for (const item of items) {
    if (!Array.isArray(item) || item.length !== 3) {
      return;
    }
    const [name, type, value] = item;
    if (
      typeof name !== 'string' ||
      !name ||
      typeof value !== 'string' ||
      typeof type !== 'number' ||
      !Object.hasOwn(matcherOperators, type)
    ) {
      return;
    }
    set.push({ name, type, value });
  }
  return set;
}

function readMatcherSets(values: string[]) {
  return values.map(decodeMatcherSet).filter((set) => set !== undefined);
}

function activeMatcherSets(sets: MatcherDefinition[][]) {
  return sets.filter((set) => set.length > 0);
}

function matcherFilterKey(values: string[]) {
  return JSON.stringify(activeMatcherSets(readMatcherSets(values)));
}

function readStates(values: string[]) {
  if (values.includes('all')) {
    return [];
  }
  const selected = silenceStates
    .filter(({ value }) => values.includes(value))
    .map(({ value }) => value);
  return selected.length ? selected : defaultStates;
}

function decodeAnnotation(encoded: string): Annotation | undefined {
  const [key, value, ...rest] = parseArray(encoded) ?? [];
  if (!rest.length && typeof key === 'string' && typeof value === 'string') {
    return [key, value];
  }
}

function decodeSort(value: string): SilenceSort | undefined {
  const [sortField, direction] = value.split(':');
  const column = sortColumns.find(({ field }) => field === sortField);
  if (column && (direction === 'asc' || direction === 'desc')) {
    return { field: column.field, direction };
  }
}

export function readSilenceFilters(params: URLSearchParams) {
  const page = Number(params.get('page'));
  const matcherMode: MatcherMode = params.get('matcher-mode') === 'all' ? 'all' : 'any';
  const annotationMode: AnnotationMode = params.get('annotation-mode') === 'or' ? 'or' : 'and';
  return {
    states: readStates(params.getAll('state')),
    matcherSets: readMatcherSets(params.getAll('matcher')),
    matcherMode,
    annotations: Array.from(new Set(params.getAll('annotation')), decodeAnnotation).filter(
      (annotation) => annotation !== undefined
    ),
    annotationMode,
    search: params.get('q') ?? '',
    sort: decodeSort(params.get('sort') ?? ''),
    page: Number.isSafeInteger(page) && page > 0 ? page : 1,
    expandedID: params.get('expanded'),
  };
}

export type SilenceFilters = ReturnType<typeof readSilenceFilters>;

const silenceResetKeys = {
  filters: ['state', 'q', 'matcher', 'matcher-mode', 'annotation', 'annotation-mode'],
  view: ['sort', 'page', 'expanded'],
};

export function resetSilenceParams(params: URLSearchParams, scope: keyof typeof silenceResetKeys) {
  const keys = silenceResetKeys[scope];
  if (!keys.some((key) => params.has(key))) {
    return;
  }
  const next = new URLSearchParams(params);
  for (const key of keys) {
    next.delete(key);
  }
  return next;
}

export function withSilenceParam(params: URLSearchParams, key: string, values: string[]) {
  const next = new URLSearchParams(params);
  next.delete(key);
  const draftOnly =
    key === 'matcher' && matcherFilterKey(params.getAll(key)) === matcherFilterKey(values);
  if (key !== 'expanded' && !draftOnly) {
    next.delete('expanded');
    if (key !== 'page') {
      next.delete('page');
    }
  }
  for (const value of values) {
    next.append(key, value);
  }
  return next;
}

export type SilenceTagFilters = {
  onMatcher: (matcher: MatcherDefinition) => void;
  onMatcherSet: (set: MatcherDefinition[]) => void;
  onAnnotations: (items: Annotation[]) => void;
};

export function silenceTagFilters(
  params: URLSearchParams,
  { matcherSets, annotations }: SilenceFilters,
  apply: (next: URLSearchParams) => void
): SilenceTagFilters {
  const applyMatcherSets = (sets: MatcherDefinition[][]) =>
    apply(
      sets === matcherSets
        ? params
        : withSilenceParam(params, 'matcher', sets.map(encodeMatcherSet))
    );

  return {
    onMatcher: (matcher) => {
      applyMatcherSets(appendMatcher(matcherSets, matcher, matcherSets.length - 1));
    },
    onMatcherSet: (matchers) => {
      const exists = matcherSets.some(
        (set) => containsMatcherSet(set, matchers) && containsMatcherSet(matchers, set)
      );
      applyMatcherSets(exists ? matcherSets : [...matcherSets, matchers]);
    },
    onAnnotations: (items) => {
      const values = new Set(annotations.map(encodeAnnotation));
      const previousSize = values.size;
      for (const item of items) {
        values.add(encodeAnnotation(item));
      }
      apply(
        values.size === previousSize ? params : withSilenceParam(params, 'annotation', [...values])
      );
    },
  };
}

export function silenceListInput({ states, matcherSets }: SilenceFilters): SilenceListInput {
  return {
    states: silenceStates.filter(({ value }) => states.includes(value)).map(({ state }) => state),
    // An empty set matches all silences on the server.
    matcherFilters: activeMatcherSets(matcherSets).map((matchers) => ({ matchers })),
  };
}

function matchesAllMatcherSets(stored: Silence['matcherSets'], filters: MatcherDefinition[][]) {
  if (stored.length < filters.length) {
    return false;
  }

  const candidates = filters.map((filter) =>
    stored.flatMap(({ matchers }, index) => (containsMatcherSet(matchers, filter) ? [index] : []))
  );
  const assignedFilters = new Map<number, number>();

  function assign(filterIndex: number, visited: Set<number>): boolean {
    for (const storedIndex of candidates[filterIndex]) {
      if (visited.has(storedIndex)) {
        continue;
      }
      visited.add(storedIndex);
      const previous = assignedFilters.get(storedIndex);
      // Try to move the previous filter to another stored set.
      if (previous === undefined || assign(previous, visited)) {
        assignedFilters.set(storedIndex, filterIndex);
        return true;
      }
    }
    return false;
  }

  return candidates.every((_, index) => assign(index, new Set()));
}

export function filterSilencesByMatcherSets(
  silences: Silence[],
  { matcherSets, matcherMode }: SilenceFilters
) {
  // ListSilences already applies Any. All only differs for multiple filter sets.
  const activeSets = matcherMode === 'all' ? activeMatcherSets(matcherSets) : [];
  return activeSets.length > 1
    ? silences.filter((silence) => matchesAllMatcherSets(silence.matcherSets, activeSets))
    : silences;
}

export function listAnnotations(silence: Silence): Annotation[] {
  return Object.entries(silence.annotations).sort(([a], [b]) => a.localeCompare(b));
}

const compareTimestamp = (a: Timestamp, b: Timestamp) =>
  a.seconds === b.seconds ? a.nanos - b.nanos : a.seconds < b.seconds ? -1 : 1;

function compareSilences(a: Silence, b: Silence, { field, direction }: SilenceSort) {
  let order: number;
  if (field === 'createdBy') {
    order = a.createdBy.localeCompare(b.createdBy);
  } else {
    const left = a[field];
    const right = b[field];
    if (!left || !right) {
      return Number(!left) - Number(!right) || a.id.localeCompare(b.id);
    }
    order = compareTimestamp(left, right);
  }
  return (direction === 'asc' ? order : -order) || a.id.localeCompare(b.id);
}

function matchesAnnotations(
  stored: Silence['annotations'],
  selected: Annotation[],
  mode: AnnotationMode
) {
  if (!selected.length) {
    return true;
  }
  const matches = ([key, value]: Annotation) => Object.hasOwn(stored, key) && stored[key] === value;
  return mode === 'or' ? selected.some(matches) : selected.every(matches);
}

export function filterSilences(
  silences: Silence[],
  { search, annotations, annotationMode, sort }: SilenceFilters
) {
  const query = search.trim().toLowerCase();
  const filtered = silences.filter(
    (silence) =>
      matchesAnnotations(silence.annotations, annotations, annotationMode) &&
      [silence.id, silence.createdBy, silence.comment].some((value) =>
        value.toLowerCase().includes(query)
      )
  );
  return sort ? filtered.sort((a, b) => compareSilences(a, b, sort)) : filtered;
}

export function annotationOptions(silences: Silence[], selected: Annotation[]) {
  const options = new Map<string, string>();
  const add = (annotation: Annotation) =>
    options.set(encodeAnnotation(annotation), formatAnnotation(annotation));
  selected.forEach(add);
  for (const silence of silences) {
    Object.entries(silence.annotations).forEach(add);
  }
  return Array.from(options, ([value, label]) => ({ value, label })).sort((a, b) =>
    a.label.localeCompare(b.label)
  );
}
