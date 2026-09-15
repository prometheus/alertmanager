import type { Transport } from '@connectrpc/connect';
import { TransportProvider } from '@connectrpc/connect-query';
import { CodeHighlightAdapterProvider, plainTextAdapter } from '@mantine/code-highlight';
import { MantineProvider } from '@mantine/core';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render as testingLibraryRender } from '@testing-library/react';

import { apiTransport } from '../src/data/connect';
import { theme } from '../src/theme';

type RenderOptions = {
  transport?: Transport;
};

export function render(ui: React.ReactNode, { transport = apiTransport }: RenderOptions = {}) {
  const queryClient = new QueryClient();
  return testingLibraryRender(ui, {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <MantineProvider theme={theme} env="test">
        <CodeHighlightAdapterProvider adapter={plainTextAdapter}>
          <QueryClientProvider client={queryClient}>
            <TransportProvider transport={transport}>{children}</TransportProvider>
          </QueryClientProvider>
        </CodeHighlightAdapterProvider>
      </MantineProvider>
    ),
  });
}
