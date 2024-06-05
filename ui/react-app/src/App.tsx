import { Box, styled } from '@mui/material';
import Navbar from './components/navbar';
import Router from './Router';

// Based on the MUI doc: https://mui.com/material-ui/react-app-bar/#fixed-placement
const Offset = styled('div')(({ theme }) => theme.mixins.toolbar);

function App() {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Navbar />
      <Offset />
      <Box
        sx={{
          flex: 1,
        }}
      >
        <Router />
      </Box>
    </Box>
  );
}

export default App;
