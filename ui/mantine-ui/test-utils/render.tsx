import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render as testingLibraryRender } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { theme } from '../src/theme';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

export function render(ui: React.ReactNode) {
  return testingLibraryRender(ui, {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={createTestQueryClient()}>
        <MantineProvider theme={theme} env="test">
          {children}
        </MantineProvider>
      </QueryClientProvider>
    ),
  });
}
