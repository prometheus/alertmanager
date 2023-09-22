import React, { useState } from 'react';
import ExpandedIcon from 'mdi-material-ui/ChevronDown';
import BellOff from 'mdi-material-ui/BellOff';
import {
  Card,
  CardActions,
  CardContent,
  Chip,
  Collapse,
  Container,
  Divider,
  IconButton,
  IconButtonProps,
  List,
  ListItem,
  styled,
  Tooltip,
  Typography,
} from '@mui/material';
import Grid from '@mui/material/Unstable_Grid2';
import { AlertGroup, useAlertGroups } from '../client/alert';

interface ExpandMoreProps extends IconButtonProps {
  expand: boolean;
}

const ExpandMore = styled((props: ExpandMoreProps) => {
  const { ...other } = props;
  return <IconButton {...other} />;
})(({ theme, expand }) => ({
  transform: !expand ? 'rotate(0deg)' : 'rotate(180deg)',
  marginLeft: 'auto',
  transition: theme.transitions.create('transform', {
    duration: theme.transitions.duration.shortest,
  }),
}));

interface AlertCardProps {
  group: AlertGroup;
}

function renderLabels(labels: Record<string, string>) {
  const result = [];
  for (const k in labels) {
    result.push(<Chip key={k} size="small" label={`${k}: ${labels[k]}`} />);
  }
  return result;
}

function AlertCard(props: AlertCardProps) {
  const { group } = props;
  const [expanded, setExpanded] = useState(false);
  const handleExpandClick = () => {
    setExpanded(!expanded);
  };

  return (
    <Card>
      <CardContent>{renderLabels(group.labels)}</CardContent>
      <CardActions disableSpacing>
        <Tooltip title="Silence this group">
          <IconButton>
            <BellOff />
          </IconButton>
        </Tooltip>
        <ExpandMore expand={expanded} onClick={handleExpandClick} aria-expanded={expanded} aria-label="show more">
          <ExpandedIcon />
        </ExpandMore>
      </CardActions>
      <Collapse in={expanded} timeout="auto" unmountOnExit>
        <Divider />
        <CardContent>
          {group.alerts.map((alert, index) => {
            return (
              <List key={`alert-${index}`}>
                <ListItem sx={{ flexWrap: 'wrap', justifyContent: 'flex-start' }}>
                  {renderLabels(alert.labels)}
                </ListItem>
              </List>
            );
          })}
        </CardContent>
      </Collapse>
    </Card>
  );
}

export default function AlertView() {
  const { data } = useAlertGroups();
  if (data === undefined || data === null) {
    return null;
  }
  return (
    <Container>
      <Typography variant="h4">Alert</Typography>
      <Grid container spacing={{ xs: 2, md: 3 }} columns={{ xs: 4, sm: 8, md: 12 }}>
        {data.map((group, index) => {
          return (
            <Grid key={index} xs={4} sm={4} md={4}>
              <AlertCard group={group} />
            </Grid>
          );
        })}
      </Grid>
    </Container>
  );
}
