import { useAlertGroups } from '../client/alert';
import { Container, Typography } from '@mui/material';

export default function AlertView() {
  const { data } = useAlertGroups();
  if (data === undefined || data === null) {
    return null;
  }

  return (
    <Container maxWidth="md">
      <Typography variant="h4">Alert</Typography>
    </Container>
  );
}
