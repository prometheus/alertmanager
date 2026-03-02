import '@mantine/core/styles.css';
import '@mantine/code-highlight/styles.css';

import { Suspense } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import hljs from 'highlight.js/lib/core';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { CodeHighlightAdapterProvider, createHighlightJsAdapter } from '@mantine/code-highlight';
import { AppShell, Box, MantineProvider, Skeleton } from '@mantine/core';
import ErrorBoundary from './components/ErrorBoundary';
import { Header } from './components/Header';
import { AlertsPage } from './pages/Alerts.page';
import { ConfigPage } from './pages/Config.page';
import { SilencesPage } from './pages/Silences.page';
import { StatusPage } from './pages/Status.page';
import { theme } from './theme';

import './highlightjs.css';

import yamlLang from 'highlight.js/lib/languages/yaml';

hljs.registerLanguage('yaml', yamlLang);
const highlightJsAdapter = createHighlightJsAdapter(hljs);

const queryClient = new QueryClient();

export default function App() {
  return (
    <BrowserRouter>
      <MantineProvider theme={theme}>
        <CodeHighlightAdapterProvider adapter={highlightJsAdapter}>
          <QueryClientProvider client={queryClient}>
            <AppShell padding="md" header={{ height: 60 }}>
              <Header />
              <AppShell.Main>
                <ErrorBoundary key={location.pathname}>
                  <Suspense
                    fallback={
                      <Box mt="lg">
                        {Array.from(Array(10), (_, i) => (
                          <Skeleton key={i} height={40} mb={15} width={1000} mx="auto" />
                        ))}
                      </Box>
                    }
                  >
                    {/* Main content will be rendered here by the Router */}
                    <Routes>
                      {/* Redirect the root path to the alerts page */}
                      {/* TODO(@sysadmind): This should take the fact that previous UI used /#/routeName */}
                      <Route path="/" element={<Navigate to="/alerts" replace />} />
                      <Route path="/alerts" element={<AlertsPage />} />
                      <Route path="/silences" element={<SilencesPage />} />
                      <Route path="/status" element={<StatusPage />} />
                      <Route path="/config" element={<ConfigPage />} />
                    </Routes>
                  </Suspense>
                </ErrorBoundary>
              </AppShell.Main>
            </AppShell>
          </QueryClientProvider>
        </CodeHighlightAdapterProvider>
      </MantineProvider>
    </BrowserRouter>
  );
}
