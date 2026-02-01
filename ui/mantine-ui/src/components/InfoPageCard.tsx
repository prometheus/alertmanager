// import { IconProps } from "@tabler/icons-react";
import { FC, ReactNode } from 'react';
import { Card, em, Group } from '@mantine/core';

const infoPageCardTitleIconStyle = { width: em(17.5), height: em(17.5) };

const InfoPageCard: FC<{
  children: ReactNode;
  title?: string;
  icon?: React.ComponentType<any>;
}> = ({ children, title, icon: Icon }) => {
  return (
    <Card shadow="xs" withBorder p="md">
      {title && (
        <Group wrap="nowrap" align="center" ml="xs" mb="sm" gap="xs" fz="xl" fw={600}>
          {Icon && <Icon style={infoPageCardTitleIconStyle} />}
          {title}
        </Group>
      )}
      {children}
    </Card>
  );
};

export default InfoPageCard;
