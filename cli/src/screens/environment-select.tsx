import React from 'react';
import { Box, Text, useInput, useApp } from 'ink';
import SelectInput from 'ink-select-input';
import { environments, Environment } from '../config/environments.js';

interface EnvironmentSelectScreenProps {
  title: string;
  onSelect: (env: Environment) => void;
  currentSelection?: string;
}

export function EnvironmentSelectScreen({
  title,
  onSelect,
  currentSelection,
}: EnvironmentSelectScreenProps) {
  const { exit } = useApp();

  useInput((input) => {
    if (input === 'q') {
      exit();
    }
  });

  const items = environments.map(env => ({
    label: `${env.name} (${env.apiUrl})${env.key === currentSelection ? ' [current]' : ''}`,
    value: env.key,
  }));

  const handleSelect = (item: { label: string; value: string }) => {
    const env = environments.find(e => e.key === item.value);
    if (env) {
      onSelect(env);
    }
  };

  return (
    <Box flexDirection="column" padding={1}>
      <Box marginBottom={1}>
        <Text bold color="cyan">Psychic Homily Admin CLI</Text>
      </Box>

      <Box marginBottom={1}>
        <Text>{title}</Text>
      </Box>

      <SelectInput items={items} onSelect={handleSelect} />

      <Box marginTop={1}>
        <Text dimColor>Use arrow keys to select, Enter to confirm, q to quit</Text>
      </Box>
    </Box>
  );
}
