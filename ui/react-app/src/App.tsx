import { Box } from '@mui/material';
import Navbar from './components/navbar';

function App() {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
      }}
    >
      <Navbar />
      <Box
        sx={{
          paddingBottom: (theme) => theme.spacing(1),
          flex: 1,
        }}
      >
      </Box>
    </Box>
  );
}

export default App;
