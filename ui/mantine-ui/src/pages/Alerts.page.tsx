import { useMemo, useState } from 'react';
import { IconPlus } from '@tabler/icons-react';
import { ActionIcon, Group, Stack, Switch, TextInput } from '@mantine/core';
import { AlertGroupList } from '@/components/AlertGroupList';
import { LabelPill } from '@/components/LabelPill';
import { useGroupParams, useGroups } from '@/data/groups';

export function AlertsPage() {
  const [filterParams, setFilterParams] = useState<useGroupParams>({
    silenced: 'false',
    inhibited: 'false',
    filter: {},
  });
  const [filter, setFilter] = useState('');
  const [filterError, setFilterError] = useState('');
  const { data, isLoading, error } = useGroups(filterParams);

  const handleSubAdd = (label: string, value: string) => {
    setFilterParams((prev) => ({
      ...prev,
      filter: {
        ...prev.filter,
        [label]: value,
      },
    }));
  };

  const groupList = useMemo(() => {
    if (!data) {
      return [];
    }
    return <AlertGroupList groups={data} addCallback={handleSubAdd} />;
  }, [data]);

  const validateFilterText = (text: string) => {
    // Basic validation: should be in the form of key=value

    // split on the first '='
    const [key, value] = text.split(/=(.+)/);
    if (!key || !value) {
      return false;
    }
    // further validation can be added here (e.g., allowed characters, etc.)
    return true;
  };

  const trim = (str: string, char: string): string => {
    if (!str || !char) {
      return str;
    }
    let start = 0;
    let end = str.length;

    while (start < end && str[start] === char) {
      start++;
    }
    while (end > start && str[end - 1] === char) {
      end--;
    }

    return str.slice(start, end);
  };

  const handleAddFilter = () => {
    const isValid = validateFilterText(filter);
    if (!isValid) {
      setFilterError('Invalid filter format. Use key=value.');
      return;
    }
    setFilterError('');
    const [key, rawValue] = filter.split(/=(.+)/);
    const value = trim(rawValue, '"');

    setFilterParams((prev) => ({
      ...prev,
      filter: {
        ...prev.filter,
        [key]: value,
      },
    }));
    setFilter('');
  };

  const handleRemoveFilter = (key: string) => {
    setFilterParams((prev) => {
      const newFilter = { ...prev.filter };
      delete newFilter[key];
      return {
        ...prev,
        filter: newFilter,
      };
    });
  };

  return (
    <Stack mt="xs">
      {/* Filter Section */}
      <Group justify="space-between">
        {/* Filter Pills */}
        <Group flex="1">
          {/* Pills for each filter */}
          {filterParams.filter &&
            Object.entries(filterParams.filter).map((entry) => (
              <LabelPill
                key={entry[0]}
                name={entry[0]}
                value={entry[1]}
                onRemove={() => handleRemoveFilter(entry[0])}
              />
            ))}
          {/* Add Filter Section */}
          <Group flex="1" miw="200px">
            <TextInput
              flex="1"
              value={filter}
              onChange={(event) => {
                setFilter(event.currentTarget.value);
                setFilterError('');
              }}
              placeholder="Filter alerts..."
              error={filterError}
              onKeyUp={(event) => {
                if (event.key === 'Enter') {
                  handleAddFilter();
                }
              }}
            />
            <ActionIcon variant="filled" aria-label="Add filter" onClick={handleAddFilter}>
              <IconPlus style={{ width: '70%', height: '70%' }} stroke={1.5} />
            </ActionIcon>
          </Group>
        </Group>
        {/* Boolean Inputs Section */}
        <Stack>
          <Switch
            checked={filterParams.silenced === 'true'}
            onChange={(event) => {
              setFilterParams((prev) => ({
                ...prev,
                silenced: event.target.checked ? 'true' : 'false',
              }));
            }}
            label="Show Silenced Alerts"
          />
          <Switch
            checked={filterParams.inhibited === 'true'}
            onChange={(event) => {
              setFilterParams((prev) => ({
                ...prev,
                inhibited: event.target.checked ? 'true' : 'false',
              }));
            }}
            label="Show Inhibited Alerts"
          />
        </Stack>
      </Group>
      {!isLoading && !error && groupList}
    </Stack>
  );
}
