import { AppBar, Box, Button, Toolbar, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';


export default function Navbar(): JSX.Element {
  const navigate = useNavigate();
  return (
    <AppBar position='relative'>
      <Toolbar>
        <Box sx={{ display: 'flex', flexDirection: 'row' }} flexGrow={1}>
          <Button
            onClick={() => {
              navigate('/');
            }}
          >
            <Typography
              variant='h1'
              sx={(theme) => ({
                marginRight: '1rem',
                color: theme.palette.common.white,
              })}
            >
              AlertManager
            </Typography>
          </Button>
        </Box>
      </Toolbar>
    </AppBar>
  );
}
