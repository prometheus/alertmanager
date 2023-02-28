// Other routes are lazy-loaded for code-splitting
import { Suspense, lazy } from 'react';
import { Route, Routes } from 'react-router-dom';

const ViewStatus = lazy(() => import('./views/ViewStatus'));

function Router() {
  return (
    <Suspense>
      <Routes>
        <Route path="/react-app/status" element={<ViewStatus />} />
      </Routes>
    </Suspense>
  );
}

export default Router;
