import { Badge, Button, Group, Paper, Pill, Stack, Text } from '@mantine/core';

import {
  type Annotation,
  formatAnnotation,
  formatMatcher,
  type MatcherDefinition,
  type SilenceTagActions,
} from '@/data/silenceFilters';
import type { MatcherSet } from '@/gen/common/v3alpha/common_pb';

import { type SilenceTagDrag, useSilenceTagDrag } from './SilenceTagDrag';
import classes from './SilenceTags.module.css';

const tagColors = [
  'blue',
  'cyan',
  'teal',
  'green',
  'lime',
  'orange',
  'red',
  'pink',
  'grape',
  'violet',
  'indigo',
];

function tagColor(text: string) {
  let hash = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    hash = Math.imul(hash ^ text.charCodeAt(i), 0x01000193);
  }
  return tagColors[(hash >>> 0) % tagColors.length];
}

export type SilenceTagProps = {
  label: string;
  onFilter?: () => void;
  drag?: SilenceTagDrag;
  wrap?: boolean;
};

export function matcherTagProps(
  matcher: MatcherDefinition,
  addMatcher?: SilenceTagActions['addMatcher']
): SilenceTagProps {
  return {
    label: formatMatcher(matcher),
    onFilter: addMatcher && (() => addMatcher(matcher)),
    drag: { kind: 'matcher', matcher },
  };
}

export function annotationTagProps(
  annotation: Annotation,
  addAnnotations: SilenceTagActions['addAnnotations']
): SilenceTagProps {
  return {
    label: formatAnnotation(annotation),
    onFilter: () => addAnnotations([annotation]),
    drag: { kind: 'annotation', annotation },
  };
}

export function SilenceTag({ label, onFilter, drag, wrap }: SilenceTagProps) {
  const action = onFilter ? `Filter by ${label}` : undefined;
  const dragProps = useSilenceTagDrag(onFilter && drag);
  const title = action
    ? `${action}${drag?.kind === 'matcher' ? '. Adds to the last matcher set' : ''}`
    : label;
  return (
    <Badge
      {...dragProps}
      component={onFilter ? 'button' : 'span'}
      type={onFilter ? 'button' : undefined}
      className={classes.tag}
      classNames={{ label: wrap ? classes.wrapLabel : undefined }}
      mod={{ wrap }}
      color={tagColor(label)}
      variant="light"
      title={dragProps ? `${title}. Drag to add to filters.` : title}
      aria-label={action}
      onClick={onFilter}
    >
      {label}
    </Badge>
  );
}

export function SilenceFilterPill({
  label,
  removeLabel,
  onRemove,
  disabled,
  drag,
}: {
  label: string;
  removeLabel: string;
  onRemove?: () => void;
  disabled?: boolean;
  drag?: SilenceTagDrag;
}) {
  const dragProps = useSilenceTagDrag(drag);
  return (
    <Pill
      {...dragProps}
      title={dragProps ? `${label}. Drag to move to another set.` : label}
      bg={`var(--mantine-color-${tagColor(label)}-light)`}
      c="var(--mantine-color-text)"
      disabled={disabled}
      withRemoveButton={!!onRemove}
      removeButtonProps={{ 'aria-hidden': false, 'aria-label': removeLabel, tabIndex: 0 }}
      onRemove={onRemove}
    >
      {label}
    </Pill>
  );
}

export function SilenceMatcherSets({
  sets,
  addMatcher,
  addMatcherSet,
}: Partial<Pick<SilenceTagActions, 'addMatcher' | 'addMatcherSet'>> & { sets: MatcherSet[] }) {
  if (!sets.length) {
    return (
      <Text size="sm" c="dimmed">
        No matchers.
      </Text>
    );
  }
  return (
    <Stack gap={4}>
      {sets.map(({ matchers }, index) => (
        <Stack key={index} gap={4}>
          {index > 0 && (
            <Text size="xs" fw={600}>
              OR
            </Text>
          )}
          <Paper withBorder px="xs" py={4} role="group" aria-label={`Matcher set ${index + 1}`}>
            <Group gap="xs" justify="space-between" mb={4}>
              <Text size="xs" fw={600}>
                Set {index + 1} (AND)
              </Text>
              {addMatcherSet && matchers.length > 0 && (
                <Button
                  type="button"
                  variant="subtle"
                  size="compact-xs"
                  title="Add this set to filters"
                  aria-label={`Add matcher set ${index + 1} to filters`}
                  onClick={() => addMatcherSet(matchers)}
                >
                  Add to filters
                </Button>
              )}
            </Group>
            <Group gap={4}>
              {matchers.map((matcher, i) => (
                <SilenceTag key={i} {...matcherTagProps(matcher, addMatcher)} wrap />
              ))}
            </Group>
          </Paper>
        </Stack>
      ))}
    </Stack>
  );
}

export function SilenceAnnotations({
  annotations,
  addAnnotations,
  compact,
}: Pick<SilenceTagActions, 'addAnnotations'> & {
  annotations: Annotation[];
  compact?: boolean;
}) {
  return (
    <Stack gap={compact ? 4 : 'sm'}>
      <Group gap="xs" justify="space-between" ml={compact ? 0 : 'xs'}>
        <Text component={compact ? 'span' : 'h2'} size={compact ? 'xs' : 'xl'} fw={600}>
          Annotations
        </Text>
        {annotations.length > 0 && (
          <Button
            type="button"
            size="compact-xs"
            variant="subtle"
            onClick={() => addAnnotations(annotations)}
          >
            Add all to filters
          </Button>
        )}
      </Group>
      {annotations.length > 0 ? (
        <Group gap={4}>
          {annotations.map((annotation) => (
            <SilenceTag
              key={annotation[0]}
              {...annotationTagProps(annotation, addAnnotations)}
              wrap
            />
          ))}
        </Group>
      ) : (
        <Text size="sm" c="dimmed">
          No annotations.
        </Text>
      )}
    </Stack>
  );
}
