import React from 'react';
import { Box, Text, useInput, useApp } from 'ink';
import { Show } from '../api/client.js';

interface ImportResultScreenProps {
  targetEnvName: string;
  results: {
    success: boolean;
    show?: Show;
    error?: string;
  }[];
  successCount: number;
  errorCount: number;
  onDone: () => void;
}

export function ImportResultScreen({
  targetEnvName,
  results,
  successCount,
  errorCount,
  onDone,
}: ImportResultScreenProps) {
  const { exit } = useApp();

  useInput((input, key) => {
    if (input === 'q') {
      exit();
      return;
    }

    if (key.return || key.escape) {
      onDone();
      return;
    }
  });

  return (
    <Box flexDirection="column" padding={1}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold color="cyan">
          Import Complete: {targetEnvName}
        </Text>
      </Box>

      {/* Summary */}
      <Box marginBottom={1}>
        <Text>
          <Text color="green">{successCount} succeeded</Text>
          {errorCount > 0 && (
            <Text>
              {', '}
              <Text color="red">{errorCount} failed</Text>
            </Text>
          )}
        </Text>
      </Box>

      {/* Results */}
      <Box flexDirection="column" marginBottom={1}>
        {results.map((result, idx) => (
          <Box key={idx}>
            {result.success ? (
              <Text>
                <Text color="green">✓</Text> {result.show?.title || `Show ${idx + 1}`}
                {result.show && (
                  <Text dimColor> (ID: {result.show.id})</Text>
                )}
              </Text>
            ) : (
              <Text>
                <Text color="red">✗</Text> Show {idx + 1}
                <Text color="red"> - {result.error}</Text>
              </Text>
            )}
          </Box>
        ))}
      </Box>

      {/* Controls */}
      <Box>
        <Text dimColor>[Enter] Continue  [q] Quit</Text>
      </Box>
    </Box>
  );
}
