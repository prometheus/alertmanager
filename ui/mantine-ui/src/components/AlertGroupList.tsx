import { Box, Card, Group, rem, Table, Text, Tooltip } from '@mantine/core';
import { Accordion } from '@/components/Accordion';
import { LabelBadges } from '@/components/LabelBadges';
import { Group as AlertGroup } from '@/data/groups';
import { parseISO8601 } from '@/lib/time';

export type AlertGroupListProps = {
  groups: Array<AlertGroup>;
  addCallback?: (label: string, value: string) => void;
};

export function AlertGroupList({ groups, addCallback }: AlertGroupListProps) {
  return groups?.map((group, i) => (
    <Card shadow="xs" withBorder p="md" key={i}>
      <Group mb="sm" justify="space-between">
        <Group align="baseline">
          <Text fz="xl" fw={600} c="var(--mantine-primary-color-filled)">
            {group.receiver.name}
          </Text>
          <LabelBadges labels={group.labels} addCallback={addCallback} />
        </Group>
        <Group>
          <Text fz="sm" c="gray.6">
            {group.alerts.length} alert{group.alerts.length !== 1 ? 's' : ''}
          </Text>
        </Group>
      </Group>
      <Accordion multiple variant="separated">
        {group.alerts.map((alert, j) => (
          <Accordion.Item mt={rem(5)} key={j} value={j.toString()}>
            <Accordion.Control styles={{ label: { paddingBlock: rem(10) } }}>
              <LabelBadges labels={alert.labels} addCallback={addCallback} />
            </Accordion.Control>
            <Accordion.Panel>
              <Table mt="lg">
                <Table.Tbody>
                  <Table.Tr>
                    <Table.Td w="15%">Status</Table.Td>
                    <Table.Td>{alert.status.state}</Table.Td>
                  </Table.Tr>
                  <Table.Tr>
                    <Table.Td w="15%">Ends</Table.Td>
                    <Table.Td>
                      <Tooltip label={parseISO8601(alert.endsAt).format()}>
                        <Box>{parseISO8601(alert.endsAt).fromNow()}</Box>
                      </Tooltip>
                    </Table.Td>
                  </Table.Tr>
                  <Table.Tr>
                    <Table.Td colSpan={2}>
                      <Text fz="md">Annotations</Text>
                      <Table mt="md" mb="xl">
                        <Table.Tbody>
                          {Object.entries(alert.annotations).map(([k, v]) => (
                            <Table.Tr key={k}>
                              <Table.Th
                                c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-4))"
                                w="15%"
                              >
                                {k}
                              </Table.Th>
                              <Table.Td>{v}</Table.Td>
                            </Table.Tr>
                          ))}
                        </Table.Tbody>
                      </Table>
                    </Table.Td>
                  </Table.Tr>
                </Table.Tbody>
              </Table>
            </Accordion.Panel>
          </Accordion.Item>
        ))}
      </Accordion>
    </Card>
  ));
}
