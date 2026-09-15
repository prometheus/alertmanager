import { render, screen } from '@testing-library/react';

import App from './App';
import headerClasses from './components/Header.module.css';

afterEach(() => {
  window.location.hash = '';
});

describe('App routing', () => {
  it('renders routes from the URL hash', () => {
    window.location.hash = '#/alerts';

    render(<App />);

    expect(screen.getByText('Alerts List')).toBeInTheDocument();
  });

  it('applies the header style class', () => {
    render(<App />);

    expect(screen.getByRole('banner')).toHaveClass(headerClasses.header);
  });
});
