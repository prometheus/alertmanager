import { AppShell, Burger, Button, Group, Menu, Text } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { Link, NavLink, Route, Routes } from 'react-router-dom';

import { AlertsPage } from '@/pages/Alerts.page';
import { SilencesPage } from '@/pages/Silences.page';

import classes from './Header.module.css';

const navLinkXPadding = 'md';

export const Header = () => {
  const [mobileMenuOpened, { open, close }] = useDisclosure(false);

  const mainNavPages = [
    {
      title: 'Alerts',
      path: '/alerts',
      // icon: <IconBellFilled style={navIconStyle} />,
      element: <AlertsPage />,
    },
    {
      title: 'Silences',
      path: '/silences',
      // icon: <IconSearch style={navIconStyle} />,
      element: <SilencesPage />,
    },
  ];

  const navLinks = (
    <>
      {mainNavPages.map((page) => (
        <Button
          key={page.path}
          component={NavLink}
          to={page.path}
          className={classes.navLink}
          // leftSection={page.icon}
          px={navLinkXPadding}
        >
          {page.title}
        </Button>
      ))}
      <Menu>
        <Routes>
          <Route
            path="/status"
            element={
              <Menu.Target>
                <Button
                  component={NavLink}
                  to="/status"
                  className={classes.navLink}
                  px={navLinkXPadding}
                >
                  Status {'>'} Runtime & Build Information
                </Button>
              </Menu.Target>
            }
          />
          <Route
            path="/config"
            element={
              <Menu.Target>
                <Button
                  component={NavLink}
                  to="/config"
                  className={classes.navLink}
                  px={navLinkXPadding}
                >
                  Status {'>'} Configuration
                </Button>
              </Menu.Target>
            }
          />
          {/* Default menu item when no status pages are selected */}
          <Route
            path="*"
            element={
              <Menu.Target>
                <Button
                  className={classes.navLink}
                  // leftSection={<IconServer style={navIconStyle} />}
                  // rightSection={<IconChevronDown style={navIconStyle} />}
                  px={navLinkXPadding}
                >
                  Status
                </Button>
              </Menu.Target>
            }
          />
        </Routes>
        <Menu.Dropdown>
          <Menu.Item key="runtime" component={NavLink} to="/status">
            Runtime & Build Information
          </Menu.Item>
          <Menu.Item key="config" component={NavLink} to="/config">
            Configuration
          </Menu.Item>
        </Menu.Dropdown>
      </Menu>
    </>
  );

  const mobileNavLinks = (
    <Menu
      opened={mobileMenuOpened}
      onChange={(opened) => (opened ? open() : close())}
      menuItemTabIndex={0}
      trapFocus={false}
      position="bottom-end"
    >
      <Menu.Target>
        <Burger opened={mobileMenuOpened} color="white" aria-label="Toggle navigation menu" />
      </Menu.Target>
      <Menu.Dropdown>
        {mainNavPages.map((page) => (
          <Menu.Item
            key={page.path}
            className={classes.menuItem}
            component={NavLink}
            to={page.path}
          >
            {page.title}
          </Menu.Item>
        ))}
        <Menu.Item className={classes.menuItem} component={NavLink} to="/status">
          Runtime & Build Information
        </Menu.Item>
        <Menu.Item className={classes.menuItem} component={NavLink} to="/config">
          Configuration
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );

  return (
    <AppShell.Header className={classes.header}>
      <Group h="100%" px="md" wrap="nowrap">
        <Group className={classes.navMain} justify="space-between" wrap="nowrap">
          <Group gap={40} wrap="nowrap">
            <Link to="/" style={{ textDecoration: 'none', color: 'white' }}>
              <Group gap={10} wrap="nowrap">
                {/* <img src={PrometheusLogo} height={30} /> */}
                <Text fz={20}>Alertmanager</Text>
              </Group>
            </Link>
            <Group gap={12} visibleFrom="sm" wrap="nowrap">
              {navLinks}
            </Group>
            <Group hiddenFrom="sm" ml="auto" wrap="nowrap">
              {mobileNavLinks}
            </Group>
          </Group>
        </Group>
      </Group>
    </AppShell.Header>
  );
};
