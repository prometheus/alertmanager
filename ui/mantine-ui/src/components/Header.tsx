import { Link, NavLink, Route, Routes } from 'react-router-dom';
import { AppShell, Button, Group, Menu, Text } from '@mantine/core';
import { AlertsPage } from '@/pages/Alerts.page';
import { SilencesPage } from '@/pages/Silences.page';
import classes from './Header.module.css';

const navLinkXPadding = 'md';

export const Header = () => {
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

  return (
    <AppShell.Header className={classes.header}>
      <Group h="100%" px="md" wrap="nowrap">
        <Group className={classes.navMain} justify="space-between" wrap="nowrap">
          <Group gap={40} wrap="nowrap">
            <Link to="/" style={{ textDecoration: 'none', color: 'white' }}>
              <Group gap={10} wrap="nowrap">
                {/* <img src={PrometheusLogo} height={30} /> */}
                <Text hiddenFrom="sm" fz={20}>
                  Alertmanager
                </Text>
                <Text visibleFrom="md" fz={20}>
                  Alertmanager
                </Text>
              </Group>
            </Link>
            <Group gap={12} visibleFrom="sm" wrap="nowrap">
              {navLinks}
            </Group>
          </Group>
        </Group>
      </Group>
    </AppShell.Header>
  );
};
