// Other routes are lazy-loaded for code-splitting
import { Suspense, lazy } from 'react';
import { Route, Routes } from 'react-router-dom';

const StatusView = lazy(() => import('./views/StatusView'));
const AlertView = lazy(() => import('./views/AlertView'));

const routePrefix = '/react-app';

function Router() {
  return (
    <Suspense>
      <Routes>
        <Route path={`${routePrefix}/status`} element={<StatusView />} />
        <Route path={`${routePrefix}/alert`} element={<AlertView />} />
      </Routes>
    </Suspense>
  );
}

export default Router;
