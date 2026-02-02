import { CodeHighlight } from '@mantine/code-highlight';
import { useStatus } from '@/data/status';

export function ConfigPage() {
  const { data } = useStatus();
  return (
    <CodeHighlight
      language="yaml"
      code={data.config.original}
      miw="50vw"
      w="fit-content"
      maw="calc(100vw - 75px)"
      mx="auto"
      mt="xs"
    />
  );
}
