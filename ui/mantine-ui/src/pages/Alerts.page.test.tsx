import { render } from '@test-utils';
import { AlertsPage } from './Alerts.page';

describe('AlertsPage', () => {
  it('renders without crashing', () => {
    render(<AlertsPage />);
  });
});
