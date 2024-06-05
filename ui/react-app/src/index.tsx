import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import { BrowserRouter } from 'react-router-dom';
import { QueryParamProvider } from 'use-query-params';
import { ReactRouter6Adapter } from 'use-query-params/adapters/react-router-6';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

function renderApp(container: Element | null) {
  if (container === null) {
    return;
  }
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        refetchOnWindowFocus: false,
        // react-query uses a default of 3 retries.
        // This sets the default to 0 retries.
        // If needed, the number of retries can be overridden in individual useQuery calls.
        retry: 0,
      },
    },
  });

  const root = ReactDOM.createRoot(container);
  root.render(
    <React.StrictMode>
      <BrowserRouter>
        <QueryClientProvider client={queryClient}>
          <QueryParamProvider adapter={ReactRouter6Adapter}>
            <App />
          </QueryParamProvider>
        </QueryClientProvider>
      </BrowserRouter>
    </React.StrictMode>
  );
}

renderApp(document.getElementById('root'));
