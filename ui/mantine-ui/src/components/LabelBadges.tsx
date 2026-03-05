// Copied from Prometheus UI
import { FC } from 'react';
import { IconPlus } from '@tabler/icons-react';
import { Group, Stack } from '@mantine/core';
import badgeClasses from './LabelBadges.module.css';

export interface LabelBadgesProps {
  labels: Record<string, string>;
  wrapper?: typeof Group | typeof Stack;
  style?: React.CSSProperties;
  addCallback?: (label: string, value: string) => void;
}

export const LabelBadges: FC<LabelBadgesProps> = ({
  labels,
  wrapper: Wrapper = Group,
  style,
  addCallback,
}) => (
  <Wrapper gap="xs">
    {Object.entries(labels).map(([k, v]) => (
      // We build our own Mantine-style badges here for performance
      // reasons (see comment in LabelBadges.module.css).
      <span key={k} className={badgeClasses.labelBadge} style={style}>
        {k}="{v}"
        {addCallback && (
          <span
            className={badgeClasses.addButton}
            role="button"
            tabIndex={0}
            onClick={() => addCallback(k, v)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                addCallback(k, v);
              }
            }}
          >
            <IconPlus size={16} />
          </span>
        )}
      </span>
    ))}
  </Wrapper>
);
