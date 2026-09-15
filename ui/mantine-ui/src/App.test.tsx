import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

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

  it('provides navigation from the mobile menu', async () => {
    const user = userEvent.setup();
    render(<App />);

    const menuButton = screen.getByRole('button', { name: 'Toggle navigation menu' });
    await user.click(menuButton);

    expect(menuButton).toHaveAttribute('aria-expanded', 'true');

    const menu = await screen.findByRole('menu', { hidden: true });
    const activeMenuItem = within(menu).getByRole('menuitem', { name: 'Alerts', hidden: true });
    expect(activeMenuItem).toHaveAttribute('aria-current', 'page');
    expect(activeMenuItem).toHaveClass(headerClasses.menuItem);
    expect(
      within(menu).getByRole('menuitem', { name: 'Silences', hidden: true })
    ).toBeInTheDocument();
    expect(
      within(menu).getByRole('menuitem', { name: 'Runtime & Build Information', hidden: true })
    ).toBeInTheDocument();
    expect(
      within(menu).getByRole('menuitem', { name: 'Configuration', hidden: true })
    ).toBeInTheDocument();
  });
});
