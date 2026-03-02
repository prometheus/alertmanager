import { Table } from '@mantine/core';
import InfoPageCard from '@/components/InfoPageCard';
import InfoPageStack from '@/components/InfoPageStack';
import { useStatus } from '@/data/status';

export function StatusPage() {
  const { data } = useStatus();
  return (
    <InfoPageStack>
      <InfoPageCard title="Build information">
        <Table layout="fixed">
          <Table.Tbody>
            <Table.Tr>
              <Table.Th>Version</Table.Th>
              <Table.Td>{data.versionInfo.version}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Revision</Table.Th>
              <Table.Td>{data.versionInfo.revision}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Branch</Table.Th>
              <Table.Td>{data.versionInfo.branch}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Build User</Table.Th>
              <Table.Td>{data.versionInfo.buildUser}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Build Date</Table.Th>
              <Table.Td>{data.versionInfo.buildDate}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Go Version</Table.Th>
              <Table.Td>{data.versionInfo.goVersion}</Table.Td>
            </Table.Tr>
          </Table.Tbody>
        </Table>
      </InfoPageCard>
      <InfoPageCard title="Runtime information">
        <Table layout="fixed">
          <Table.Tbody>
            <Table.Tr>
              <Table.Th>Uptime</Table.Th>
              <Table.Td>{data.uptime}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Cluster Name</Table.Th>
              <Table.Td>{data.cluster.name}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Cluster Status</Table.Th>
              <Table.Td>{data.cluster.status}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>Number of Peers</Table.Th>
              <Table.Td>{data.cluster.peers.length}</Table.Td>
            </Table.Tr>
          </Table.Tbody>
        </Table>
      </InfoPageCard>
      <InfoPageCard title="Cluster Peers">
        <Table layout="fixed">
          <Table.Tbody>
            <Table.Tr>
              <Table.Th>Peer Name</Table.Th>
              <Table.Th>Address</Table.Th>
            </Table.Tr>
            {data.cluster.peers.map((peer, index) => (
              <Table.Tr key={index}>
                <Table.Td>{peer.name}</Table.Td>
                <Table.Td>{peer.address}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </InfoPageCard>
    </InfoPageStack>
  );
}
