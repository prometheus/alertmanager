import React from 'react';
import { Pill, PillProps } from '@mantine/core';
import classes from './LabelPill.module.css';

export interface LabelPillProps extends PillProps {
  name: string;
  value: string;
}

export const LabelPill = ({ name, value, ...others }: LabelPillProps) => {
  return (
    <Pill className={classes.label} classNames={{ remove: classes.remove }} size="lg" {...others}>
      {name}="{value}"
    </Pill>
  );
};
