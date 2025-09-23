import '@mantine/core/styles.css';

import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AppShell, MantineProvider } from '@mantine/core';
import { Header } from './components/Header';
import { AlertsPage } from './pages/Alerts.page';
import { SilencesPage } from './pages/Silences.page';
import { theme } from './theme';

export default function App() {
  return (
    <BrowserRouter>
      <MantineProvider theme={theme}>
        <AppShell padding="md" header={{ height: 60 }}>
          <Header />
          <AppShell.Main>
            {/* Main content will be rendered here by the Router */}
            <Routes>
              {/* Redirect the root path to the alerts page */}
              {/* TODO(@sysadmind): This should take the fact that previous UI used /#/routeName */}
              <Route path="/" element={<Navigate to="/alerts" replace />} />
              <Route path="/alerts" element={<AlertsPage />} />
              <Route path="/silences" element={<SilencesPage />} />
            </Routes>
          </AppShell.Main>
        </AppShell>
      </MantineProvider>
    </BrowserRouter>
  );
}
