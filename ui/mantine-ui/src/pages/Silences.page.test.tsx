import type { MessageInitShape } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createRouterTransport } from '@connectrpc/connect';
import { act, fireEvent, render, screen, waitFor, within } from '@test-utils';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';

import { encodeAnnotation, encodeMatcherSet } from '@/data/silenceFilters';
import { MatcherType } from '@/gen/common/v3alpha/common_pb';
import {
  type ListSilencesRequest,
  type SilenceSchema,
  SilenceService,
  SilenceState,
} from '@/gen/silence/v3alpha/silence_pb';

import { SilencePage } from './Silence.page';
import { SilencesPage } from './Silences.page';

type TestSilence = Omit<MessageInitShape<typeof SilenceSchema>, '$typeName'>;

const timestamp = (date: string) => timestampFromDate(new Date(date));
const serviceMatcher = { name: 'service', type: MatcherType.EQUAL, value: 'api' };
const headerHeight = 36;
const rowHeight = 58;

function mockTableMeasurements() {
  let resizeResults: (height: number, width?: number) => void;
  vi.stubGlobal(
    'ResizeObserver',
    class extends ResizeObserver {
      constructor(private callback: ResizeObserverCallback) {
        super(callback);
      }

      observe(target: Element) {
        const report = (height: number, width = 1280) =>
          this.callback(
            [{ target, contentRect: new DOMRect(0, 0, width, height) } as ResizeObserverEntry],
            this
          );
        if (target.tagName === 'THEAD') {
          report(headerHeight);
        } else if (target.hasAttribute('data-expanded-row')) {
          report(rowHeight + 1);
        } else if (target.tagName === 'TR') {
          report(rowHeight);
        } else if (target.getAttribute('aria-label') === 'Silence results') {
          resizeResults = report;
          report(headerHeight + 5 * rowHeight);
        }
      }
    }
  );
  return (height: number, width?: number) => act(() => resizeResults(height, width));
}

afterEach(() => vi.unstubAllGlobals());

function silence(id: string, comment: string, overrides?: TestSilence): TestSilence {
  return {
    id,
    comment,
    createdBy: 'operator',
    status: { state: SilenceState.ACTIVE },
    startsAt: timestamp('2026-11-20T12:00:00Z'),
    endsAt: timestamp('2026-11-20T18:00:00Z'),
    updatedAt: timestamp('2026-11-20T11:00:00Z'),
    matcherSets: [{ matchers: [serviceMatcher] }],
    ...overrides,
  };
}

function setup(path: string, silences: TestSilence[]) {
  const resizeResults = mockTableMeasurements();
  const listSilences = vi.fn(({ states }: ListSilencesRequest) => ({
    silences: silences.filter((item) => !states.length || states.includes(item.status?.state ?? 0)),
  }));
  const transport = createRouterTransport(({ service }) => {
    service(SilenceService, { listSilences });
  });
  const router = createMemoryRouter(
    [
      { path: '/silences', element: <SilencesPage /> },
      { path: '/silences/:id', element: <SilencePage /> },
    ],
    { initialEntries: [path] }
  );
  render(<RouterProvider router={router} />, { transport });
  return { listSilences, router, resizeResults, user: userEvent.setup() };
}

const rowNames = () =>
  within(screen.getByRole('table', { name: 'Silences' }))
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('link')[0].textContent);

function startTagDrag(source: HTMLElement) {
  const data = new Map<string, string>();
  const dataTransfer = {
    dropEffect: 'none',
    get types() {
      return [...data.keys()];
    },
    setData: (type: string, value: string) => data.set(type, value),
    getData: (type: string) => data.get(type) ?? '',
  };
  expect(source).toHaveAttribute('draggable', 'true');
  fireEvent.dragStart(source, { dataTransfer });
  return { dataTransfer };
}

describe('Silences', () => {
  it('sorts by last update time', async () => {
    const { user, listSilences } = setup('/silences', [
      silence('middle', 'Silence B', { updatedAt: timestamp('2026-11-18T12:00:00Z') }),
      silence('old', 'Silence A', { updatedAt: timestamp('2026-11-17T12:00:00Z') }),
      silence('new', 'Silence C', { updatedAt: timestamp('2026-11-19T12:00:00Z') }),
    ]);

    await screen.findByText('1–3 of 3 silences');
    const controls = within(screen.getByRole('group', { name: 'Silence list controls' }));
    const updated = screen.getByRole('button', { name: 'Last updated' });
    const sort = controls.getByRole('combobox', { name: 'Sort silences' });
    const reset = controls.getByRole('button', { name: 'Reset view' });
    const refresh = controls.getByRole('button', { name: 'Refresh' });
    expect(reset.nextElementSibling).toBe(refresh);
    expect(reset).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Reset filters' })).toBeDisabled();
    expect(controls.getByRole('button', { name: 'New silence' })).toBeDisabled();
    expect(listSilences).toHaveBeenCalledWith(
      expect.objectContaining({ states: [SilenceState.ACTIVE, SilenceState.PENDING] }),
      expect.anything()
    );
    expect(rowNames()).toEqual(['Silence B', 'Silence A', 'Silence C']);

    await user.click(updated);
    expect(rowNames()).toEqual(['Silence C', 'Silence B', 'Silence A']);
    expect(updated.closest('th')).toHaveAttribute('aria-sort', 'descending');
    await user.click(updated);
    expect(rowNames()).toEqual(['Silence A', 'Silence B', 'Silence C']);
    expect(updated.closest('th')).toHaveAttribute('aria-sort', 'ascending');
    expect(sort).toHaveValue('updatedAt:asc');

    await user.click(reset);
    expect(reset).toBeDisabled();
    expect(rowNames()).toEqual(['Silence B', 'Silence A', 'Silence C']);
    expect(listSilences).toHaveBeenCalledTimes(1);

    await user.click(refresh);
    await waitFor(() => expect(listSilences).toHaveBeenCalledTimes(2));
  });

  it('resets filters and view settings independently', async () => {
    const filters = new URLSearchParams({
      state: 'all',
      q: 'Silence',
      matcher: encodeMatcherSet([serviceMatcher]),
      'matcher-mode': 'all',
      annotation: '["team","ops"]',
      'annotation-mode': 'or',
    });
    const view = 'sort=updatedAt%3Aasc&page=2&expanded=row-5';
    const { user, listSilences, router } = setup(
      `/silences?${filters}&${view}`,
      Array.from({ length: 6 }, (_, i) =>
        silence(`row-${i}`, `Silence ${i + 1}`, { annotations: { team: 'ops' } })
      )
    );
    await screen.findByRole('region', { name: 'Expanded silence tags' });
    const filterBox = within(screen.getByRole('group', { name: 'Filters' }));
    const resetFilters = filterBox.getByRole('button', { name: 'Reset filters' });
    const resetView = screen.getByRole('button', { name: 'Reset view' });

    await user.click(resetFilters);
    expect(router.state.location.search).toBe(`?${view}`);
    const mode = within(screen.getByRole('radiogroup', { name: 'Matcher set filter mode' }));
    expect(mode.getByRole('radio', { name: 'Any' })).toBeChecked();
    expect(resetFilters).toBeDisabled();
    expect(resetView).toBeEnabled();
    await waitFor(() =>
      expect(listSilences).toHaveBeenLastCalledWith(
        expect.objectContaining({
          states: [SilenceState.ACTIVE, SilenceState.PENDING],
          matcherFilters: [],
        }),
        expect.anything()
      )
    );

    await act(() => router.navigate(-1));
    await user.click(resetView);
    expect(router.state.location.search).toBe(`?${filters}`);
    expect(resetView).toBeDisabled();
    expect(resetFilters).toBeEnabled();
    expect(screen.queryByRole('region', { name: 'Expanded silence tags' })).not.toBeInTheDocument();
  });

  it('sends states and matcher sets to Connect', async () => {
    const { user, listSilences } = setup('/silences', [
      silence('archive', 'Previous maintenance', { status: { state: SilenceState.EXPIRED } }),
    ]);

    expect(await screen.findByText('No silences match')).toBeInTheDocument();
    const state = within(screen.getByRole('group', { name: 'State' }));
    await user.click(state.getByRole('checkbox', { name: 'All' }));
    await user.click(state.getByRole('checkbox', { name: 'Expired' }));

    const matcher = await screen.findByRole('button', { name: 'Filter by service="api"' });
    await user.click(matcher);
    await user.click(screen.getByRole('button', { name: 'Add matcher to set 1' }));
    await user.type(screen.getByRole('textbox', { name: 'Matcher name' }), 'job');
    await user.selectOptions(screen.getByRole('combobox', { name: 'Operator' }), '=~');
    await user.type(screen.getByRole('textbox', { name: 'Matcher value' }), 'batch|worker');
    await user.click(screen.getByRole('button', { name: 'Apply matcher' }));
    const requestCount = listSilences.mock.calls.length;
    const matcherSets = within(screen.getByRole('region', { name: 'Matcher sets' }));
    await user.click(matcherSets.getByRole('button', { name: 'Add matcher set' }));
    const second = within(screen.getByRole('group', { name: 'Matcher filter set 2' }));
    expect(second.getByText('No matchers')).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Matcher name' })).not.toBeInTheDocument();
    expect(listSilences).toHaveBeenCalledTimes(requestCount);

    await user.click(second.getByRole('button', { name: 'Add matcher to set 2' }));
    await user.type(screen.getByRole('textbox', { name: 'Matcher name' }), 'env');
    await user.type(screen.getByRole('textbox', { name: 'Matcher value' }), 'prod');
    await user.click(screen.getByRole('button', { name: 'Apply matcher' }));

    await waitFor(() =>
      expect(listSilences).toHaveBeenLastCalledWith(
        expect.objectContaining({
          states: [SilenceState.EXPIRED],
          matcherFilters: [
            expect.objectContaining({
              matchers: [
                expect.objectContaining(serviceMatcher),
                expect.objectContaining({
                  name: 'job',
                  type: MatcherType.REGEXP,
                  value: 'batch|worker',
                }),
              ],
            }),
            expect.objectContaining({
              matchers: [expect.objectContaining({ name: 'env', value: 'prod' })],
            }),
          ],
        }),
        expect.anything()
      )
    );
    await user.click(screen.getByRole('button', { name: 'Remove matcher env="prod"' }));
    expect(screen.queryByRole('group', { name: 'Matcher filter set 2' })).not.toBeInTheDocument();
  });

  it('adds and removes an empty matcher set without changing the request or page', async () => {
    const params = new URLSearchParams({ page: '2', matcher: encodeMatcherSet([serviceMatcher]) });
    const { user, listSilences, router } = setup(
      `/silences?${params}`,
      Array.from({ length: 6 }, (_, i) => silence(`row-${i}`, `Silence ${i + 1}`))
    );
    await screen.findByText('6–6 of 6 silences');
    await user.click(screen.getByRole('button', { name: 'Add matcher set' }));
    const draft = within(screen.getByRole('group', { name: 'Matcher filter set 2' }));
    expect(draft.getByText('No matchers')).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Matcher name' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');

    await user.click(draft.getByRole('button', { name: 'Remove matcher set 2' }));
    expect(screen.queryByRole('group', { name: 'Matcher filter set 2' })).not.toBeInTheDocument();
    expect(router.state.location.search).toBe(`?${params}`);
    expect(listSilences).toHaveBeenCalledTimes(1);
  });

  it('matches all filter sets to different stored sets before pagination', async () => {
    const env = { name: 'env', type: MatcherType.EQUAL, value: 'prod' };
    const team = { name: 'team', type: MatcherType.EQUAL, value: 'ops' };
    const params = new URLSearchParams([
      ['matcher', encodeMatcherSet([serviceMatcher])],
      ['matcher', '[]'],
      ['matcher', encodeMatcherSet([serviceMatcher, env])],
    ]);
    const { user, listSilences, resizeResults, router } = setup(`/silences?${params}`, [
      silence('combined', 'Combined set', {
        matcherSets: [{ matchers: [serviceMatcher, env] }, { matchers: [team] }],
        annotations: { team: 'platform' },
      }),
      silence('different', 'Different operator', {
        matcherSets: [
          { matchers: [serviceMatcher, { ...env, type: MatcherType.NOT_EQUAL }] },
          { matchers: [serviceMatcher] },
          { matchers: [env] },
        ],
      }),
      silence('separate', 'Separate sets', {
        matcherSets: [
          { matchers: [serviceMatcher, env, team] },
          { matchers: [serviceMatcher] },
          { matchers: [team] },
        ],
        annotations: { team: 'ops' },
      }),
    ]);
    await screen.findByText('1–3 of 3 silences');
    const mode = within(screen.getByRole('radiogroup', { name: 'Matcher set filter mode' }));
    const sets = within(screen.getByRole('group', { name: 'Matcher filters' }));
    expect(mode.getByRole('radio', { name: 'Any' })).toBeChecked();
    expect(sets.getByText('OR')).toBeInTheDocument();
    await resizeResults(headerHeight + 2 * rowHeight);
    await user.click(await screen.findByRole('button', { name: 'Page 2' }));

    await user.click(mode.getByRole('radio', { name: 'All' }));
    expect(rowNames()).toEqual(['Separate sets']);
    expect(sets.getByText('AND')).toBeInTheDocument();
    expect(router.state.location.search).toBe(`?${params}&matcher-mode=all`);
    await user.click(screen.getByRole('combobox', { name: 'Annotations' }));
    expect(await screen.findByRole('option', { name: 'team: ops' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'team: platform' })).not.toBeInTheDocument();

    await user.click(mode.getByRole('radio', { name: 'Any' }));
    await screen.findByText('1–2 of 3 silences');
    expect(router.state.location.search).toBe(`?${params}`);
    expect(listSilences).toHaveBeenCalledTimes(1);
  });

  it('shows only complete rows and updates the page count when the table area resizes', async () => {
    const { user, listSilences, resizeResults } = setup(
      '/silences',
      Array.from({ length: 7 }, (_, i) => silence(`row-${i}`, `Silence ${i + 1}`))
    );
    await screen.findByText('1–5 of 7 silences');
    expect(rowNames()).toHaveLength(5);

    await resizeResults(headerHeight + 3.5 * rowHeight);
    await screen.findByText('1–3 of 7 silences');
    expect(rowNames()).toEqual(['Silence 1', 'Silence 2', 'Silence 3']);
    await user.click(screen.getByRole('button', { name: 'Page 3' }));
    expect(rowNames()).toEqual(['Silence 7']);
    expect(listSilences).toHaveBeenCalledTimes(1);
  });

  it('expands both tag groups in place and keeps the full page available when collapsed', async () => {
    const { user, listSilences, resizeResults, router } = setup(
      '/silences?page=2',
      Array.from({ length: 10 }, (_, i) =>
        silence(`row-${i}`, `Silence ${i + 1}`, {
          matcherSets: [
            { matchers: [serviceMatcher] },
            { matchers: [{ name: 'job', type: MatcherType.NOT_EQUAL, value: 'batch' }] },
          ],
          annotations: { env: 'prod', team: 'storage', ticket: 'OPS-42' },
        })
      )
    );
    await screen.findByText('6–10 of 10 silences');
    await resizeResults(headerHeight + 5 * rowHeight, 360);
    const last = screen.getByRole('link', { name: 'Silence 10' }).closest('tr')!;
    const more = within(last).getByRole('button', { name: 'Show 2 matcher sets' });
    await user.click(more);

    const expanded = await screen.findByRole('region', { name: 'Expanded silence tags' });
    const matchers = within(expanded).getByRole('group', { name: 'Matcher set 2' });
    expect(more).toHaveAttribute('aria-expanded', 'true');
    expect(more).toHaveAttribute('aria-controls', expanded.id);
    expect(within(last).getByRole('button', { name: 'Hide tags' })).toHaveAttribute(
      'aria-controls',
      expanded.id
    );
    expect(expanded.closest('td')?.colSpan).toBe(last.cells.length);
    expect(
      within(matchers).getByRole('button', { name: 'Filter by job!="batch"' })
    ).toBeInTheDocument();
    expect(
      within(expanded).getByRole('button', { name: 'Filter by ticket: OPS-42' })
    ).toBeInTheDocument();
    expect(within(expanded).getByText('OR')).toBeInTheDocument();
    await screen.findByText('8–10 of 10 silences');
    expect(router.state.location.pathname).toBe('/silences');
    expect(screen.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');

    await user.click(within(last).getByRole('button', { name: 'Hide annotation details' }));
    expect(screen.queryByRole('region', { name: 'Expanded silence tags' })).not.toBeInTheDocument();
    expect(rowNames()).toEqual(['Silence 6', 'Silence 7', 'Silence 8', 'Silence 9', 'Silence 10']);
    expect(listSilences).toHaveBeenCalledTimes(1);
  });

  it('adds clicked matchers to the last filter set, including an empty draft', async () => {
    const job = { name: 'job', type: MatcherType.EQUAL, value: 'worker' };
    const region = { name: 'region', type: MatcherType.EQUAL, value: 'eu' };
    const { user, listSilences, router } = setup('/silences', [
      silence('multi', 'Multiple sets', {
        matcherSets: [{ matchers: [serviceMatcher, region] }, { matchers: [job] }],
      }),
    ]);

    await user.click(await screen.findByRole('button', { name: 'Show 2 matcher sets' }));
    await user.click(screen.getByRole('button', { name: 'Add matcher set 1 to filters' }));
    await user.click(screen.getByRole('button', { name: 'Add matcher set' }));
    await user.click(await screen.findByRole('button', { name: 'Filter by service="api"' }));
    const lastSet = within(screen.getByRole('group', { name: 'Matcher filter set 2' }));
    expect(lastSet.getByText('service="api"')).toBeInTheDocument();

    await user.click(await screen.findByRole('button', { name: 'Show 2 matcher sets' }));
    await user.click(screen.getByRole('button', { name: 'Filter by job="worker"' }));

    await waitFor(() =>
      expect(listSilences).toHaveBeenLastCalledWith(
        expect.objectContaining({
          matcherFilters: [
            expect.objectContaining({
              matchers: [expect.objectContaining(serviceMatcher), expect.objectContaining(region)],
            }),
            expect.objectContaining({
              matchers: [expect.objectContaining(serviceMatcher), expect.objectContaining(job)],
            }),
          ],
        }),
        expect.anything()
      )
    );
    await user.click(lastSet.getByRole('button', { name: 'Remove matcher set 2' }));
    expect(new URLSearchParams(router.state.location.search).getAll('matcher')).toEqual([
      encodeMatcherSet([serviceMatcher, region]),
    ]);
  });

  it('copies a dragged matcher into the filter set that receives it', async () => {
    const job = { name: 'job', type: MatcherType.NOT_EQUAL, value: 'batch' };
    const params = new URLSearchParams([
      ['matcher', encodeMatcherSet([serviceMatcher])],
      ['matcher', '[]'],
    ]);
    const { user, listSilences } = setup(`/silences?${params}`, [
      silence('multi', 'Multiple sets', {
        matcherSets: [{ matchers: [serviceMatcher] }, { matchers: [job] }],
      }),
    ]);

    await user.click(await screen.findByRole('button', { name: 'Show 2 matcher sets' }));
    const drag = startTagDrag(screen.getByRole('button', { name: 'Filter by job!="batch"' }));
    const annotations = screen.getByRole('combobox', { name: 'Annotations' });
    expect(fireEvent.dragOver(annotations, drag)).toBe(true);
    const draft = screen.getByRole('group', { name: 'Matcher filter set 2' });
    expect(fireEvent.dragOver(draft, drag)).toBe(false);
    expect(drag.dataTransfer.dropEffect).toBe('copy');
    fireEvent.drop(draft, drag);

    expect(within(draft).getByText('job!="batch"')).toBeInTheDocument();
    await waitFor(() =>
      expect(listSilences).toHaveBeenLastCalledWith(
        expect.objectContaining({
          matcherFilters: [
            expect.objectContaining({ matchers: [expect.objectContaining(serviceMatcher)] }),
            expect.objectContaining({ matchers: [expect.objectContaining(job)] }),
          ],
        }),
        expect.anything()
      )
    );
  });

  it('moves a selected matcher and keeps the empty source set as a draft', async () => {
    const job = { name: 'job', type: MatcherType.NOT_EQUAL, value: 'batch' };
    const params = new URLSearchParams([
      ['matcher', encodeMatcherSet([serviceMatcher])],
      ['matcher', encodeMatcherSet([serviceMatcher])],
      ['matcher', encodeMatcherSet([job])],
    ]);
    const { listSilences, router } = setup(`/silences?${params}`, [silence('one', 'Silence')]);
    await screen.findByText('1–1 of 1 silences');
    const source = screen.getByRole('group', { name: 'Matcher filter set 1' });
    const duplicate = screen.getByRole('group', { name: 'Matcher filter set 2' });
    const target = screen.getByRole('group', { name: 'Matcher filter set 3' });
    const pill = within(source).getByText('service="api"').closest<HTMLElement>('[draggable]')!;

    const drag = startTagDrag(pill);
    expect(fireEvent.dragOver(source, drag)).toBe(true);
    expect(fireEvent.dragOver(duplicate, drag)).toBe(true);
    expect(fireEvent.dragOver(target, drag)).toBe(false);
    expect(drag.dataTransfer.dropEffect).toBe('move');
    fireEvent.drop(target, drag);

    expect(within(source).getByText('No matchers')).toBeInTheDocument();
    expect(within(target).getByText('service="api"')).toBeInTheDocument();
    expect(new URLSearchParams(router.state.location.search).getAll('matcher')).toEqual([
      '[]',
      encodeMatcherSet([serviceMatcher]),
      encodeMatcherSet([job, serviceMatcher]),
    ]);
    await waitFor(() =>
      expect(listSilences).toHaveBeenLastCalledWith(
        expect.objectContaining({
          matcherFilters: [
            expect.objectContaining({ matchers: [expect.objectContaining(serviceMatcher)] }),
            expect.objectContaining({
              matchers: [expect.objectContaining(job), expect.objectContaining(serviceMatcher)],
            }),
          ],
        }),
        expect.anything()
      )
    );
  });

  it('filters annotations before pagination and preserves the view when returning from details', async () => {
    const storageID = '24e0ecac-1986-4abf-80d7-2085b3c34ee8';
    const { user, listSilences, router } = setup('/silences?page=2', [
      ...Array.from({ length: 5 }, (_, i) =>
        silence(`dev-${i}`, `Development ${i}`, { annotations: { team: 'storage', env: 'dev' } })
      ),
      silence('platform', 'Platform maintenance', {
        annotations: { team: 'platform', env: 'prod' },
      }),
      silence(storageID, 'Storage maintenance', {
        annotations: { team: 'storage', env: 'prod', ticket: 'OPS-42' },
      }),
      silence('billing', 'Billing maintenance', { annotations: { team: 'billing', env: 'prod' } }),
    ]);

    await screen.findByText('6–8 of 8 silences');
    const annotationSection = within(screen.getByRole('region', { name: 'Annotations' }));
    const annotations = annotationSection.getByRole('combobox', { name: 'Annotations' });
    await user.type(annotations, 'team: storage');
    await user.click(await screen.findByRole('option', { name: 'team: storage' }));
    await user.click(screen.getByRole('button', { name: 'Page 2' }));
    const storage = screen.getByRole('link', { name: storageID }).closest('tr')!;
    const drag = startTagDrag(within(storage).getByRole('button', { name: 'Filter by env: prod' }));
    expect(fireEvent.dragOver(annotations, drag)).toBe(false);
    fireEvent.drop(annotations, drag);
    expect(rowNames()).toEqual(['Storage maintenance']);
    expect(new URLSearchParams(router.state.location.search).has('page')).toBe(false);

    const mode = within(screen.getByRole('radiogroup', { name: 'Annotation match mode' }));
    expect(mode.getByRole('radio', { name: 'AND' })).toBeChecked();
    await user.click(mode.getByRole('radio', { name: 'OR' }));
    await screen.findByText('1–5 of 8 silences');
    await user.click(screen.getByRole('button', { name: 'Page 2' }));
    const search = router.state.location.search;
    expect(new URLSearchParams(search).get('annotation-mode')).toBe('or');

    await user.click(screen.getByRole('link', { name: storageID }));
    const detailAnnotations = await screen.findByRole('region', { name: 'Annotations' });
    expect(
      within(detailAnnotations).getByRole('button', { name: 'Filter by ticket: OPS-42' })
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled();
    await user.click(screen.getByRole('link', { name: 'Back to silences' }));

    expect(router.state.location.search).toBe(search);
    await screen.findByText('6–8 of 8 silences');
    expect(rowNames()).toEqual([
      'Platform maintenance',
      'Storage maintenance',
      'Billing maintenance',
    ]);
    await user.type(screen.getByRole('textbox', { name: 'Text Search' }), 'STORAGE');
    expect(rowNames()).toEqual(['Storage maintenance']);
    expect(listSilences).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('link', { name: 'Storage maintenance' }));
    await user.click(await screen.findByRole('button', { name: 'Filter by ticket: OPS-42' }));
    expect(router.state.location.pathname).toBe('/silences');
    expect(new URLSearchParams(router.state.location.search).getAll('annotation')).toContain(
      '["ticket","OPS-42"]'
    );

    await user.type(screen.getByRole('combobox', { name: 'Annotations' }), 'team: platform');
    await user.click(await screen.findByRole('option', { name: 'team: platform' }));
    await user.click(screen.getByRole('radio', { name: 'AND' }));
    expect(screen.getByText('No silences match')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Remove annotation team: platform' }));
    expect(rowNames()).toEqual(['Storage maintenance']);
  });

  it.each([
    { view: 'list', path: '/silences' },
    { view: 'detail', path: '/silences/storage' },
  ])('adds all annotations to filters from the $view view', async ({ path, view }) => {
    const params = new URLSearchParams({
      annotation: encodeAnnotation(['team', 'storage']),
      'annotation-mode': 'or',
    });
    const { user, router } = setup(`${path}?${params}`, [
      silence('storage', 'Storage maintenance', {
        matcherSets: [
          { matchers: [serviceMatcher, { name: 'env', type: MatcherType.EQUAL, value: 'prod' }] },
        ],
        annotations: { env: 'prod', team: 'storage' },
      }),
    ]);

    if (view === 'list') {
      // Both previews fit. Use the row disclosure to reach the bulk actions.
      await user.click(await screen.findByRole('button', { name: 'Show tags' }));
      const panel = await screen.findByRole('region', { name: 'Expanded silence tags' });
      expect(
        within(panel).getByRole('button', { name: 'Add matcher set 1 to filters' })
      ).toBeVisible();
    }
    await user.click(await screen.findByRole('button', { name: 'Add all to filters' }));
    expect(router.state.location.pathname).toBe('/silences');
    const updated = new URLSearchParams(router.state.location.search);
    expect(updated.getAll('annotation')).toEqual([
      encodeAnnotation(['team', 'storage']),
      encodeAnnotation(['env', 'prod']),
    ]);
    expect(updated.get('annotation-mode')).toBe('or');
    expect(updated.has('expanded')).toBe(false);
  });

  it('reads an unfiltered list for direct links and reports a missing ID', async () => {
    const { listSilences, router } = setup('/silences/archive?state=active', [
      silence('archive', 'Previous maintenance', {
        status: { state: SilenceState.EXPIRED },
        matcherSets: [
          { matchers: [serviceMatcher] },
          { matchers: [{ name: 'service', type: MatcherType.NOT_REGEXP, value: 'worker.*' }] },
        ],
        receiverMatcherSets: [
          { matchers: [{ name: 'team', type: MatcherType.EQUAL, value: 'ops' }] },
        ],
      }),
    ]);

    expect(await screen.findByText('Previous maintenance')).toBeInTheDocument();
    expect(screen.getByText('service!~"worker.*"')).toBeInTheDocument();
    expect(screen.getByText('team="ops"')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Recreate' })).toBeDisabled();
    expect(listSilences).toHaveBeenCalledWith(
      expect.objectContaining({ states: [] }),
      expect.anything()
    );

    await act(() => router.navigate('/silences/unknown'));
    expect(await screen.findByText('Silence not found')).toBeInTheDocument();
  });
});
