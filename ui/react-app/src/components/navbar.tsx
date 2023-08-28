import { AppBar, Box, Button, Stack, Toolbar, Typography } from '@mui/material';
import { useLocation, useNavigate } from 'react-router-dom';

export default function Navbar(): JSX.Element {
  const navigate = useNavigate();
  const location = useLocation();
  return (
    <AppBar position={'fixed'} elevation={1}>
      <Toolbar
        sx={{
          backgroundColor: 'aliceblue',
        }}
      >
        <Box sx={{ display: 'flex', flexDirection: 'row' }} flexGrow={1}>
          <Button
            onClick={() => {
              navigate('/react-app');
            }}
          >
            <Typography
              variant="h6"
              sx={(theme) => ({
                marginRight: '1rem',
                color: theme.palette.common.black,
              })}
            >
              AlertManager
            </Typography>
          </Button>
          <Button variant="text">Alerts</Button>
          <Button variant="text">Silences</Button>
          <Button
            variant="text"
            onClick={() => {
              navigate('/react-app/status');
            }}
            disabled={location.pathname === '/react-app/status'}
          >
            Status
          </Button>
          <Button variant="text">Settings</Button>
          <Button variant="text" target="_blank" href="https://prometheus.io/docs/alerting/latest/alertmanager/">
            Help
          </Button>
        </Box>
        <Stack direction={'row'} alignItems={'center'}>
          <Button variant="outlined">New Silence</Button>
        </Stack>
      </Toolbar>
    </AppBar>
  );
}
