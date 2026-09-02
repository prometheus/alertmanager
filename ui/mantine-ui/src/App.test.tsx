import { render, screen } from '@testing-library/react';

import App from './App';

afterEach(() => {
  window.location.hash = '';
});

describe('App routing', () => {
  it('renders routes from the URL hash', () => {
    window.location.hash = '#/alerts';

    render(<App />);

    expect(screen.getByText('Alerts List')).toBeInTheDocument();
  });
});
