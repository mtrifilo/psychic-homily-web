import React, { useState, useEffect, useCallback } from 'react';
import { Box, Text, useInput, useApp } from 'ink';
import Spinner from 'ink-spinner';
import { ApiClient, Show, ApiError } from '../api/client.js';

interface ShowListScreenProps {
  sourceClient: ApiClient;
  sourceEnvName: string;
  targetEnvName: string;
  onExport: (selectedShows: Show[], exportedData: string[]) => void;
  onChangeTarget: () => void;
  onBack: () => void;
}

export function ShowListScreen({
  sourceClient,
  sourceEnvName,
  targetEnvName,
  onExport,
  onChangeTarget,
  onBack,
}: ShowListScreenProps) {
  const { exit } = useApp();
  const [shows, setShows] = useState<Show[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [cursor, setCursor] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [isExporting, setIsExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [statusFilter, setStatusFilter] = useState<string>('');
  const limit = 20;

  const loadShows = useCallback(async () => {
    setIsLoading(true);
    setError(null);

    try {
      const response = await sourceClient.getAdminShows({
        limit,
        offset,
        status: statusFilter || undefined,
      });
      setShows(response.shows);
      setTotal(response.total);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(`Failed to load shows: ${err.statusText}`);
      } else {
        setError('Failed to load shows');
      }
    } finally {
      setIsLoading(false);
    }
  }, [sourceClient, offset, statusFilter]);

  useEffect(() => {
    loadShows();
  }, [loadShows]);

  const handleExport = async () => {
    const selectedShows = shows.filter(s => selected.has(s.id));
    if (selectedShows.length === 0) {
      setError('No shows selected');
      return;
    }

    setIsExporting(true);
    setError(null);

    try {
      const response = await sourceClient.bulkExportShows(
        selectedShows.map(s => s.id)
      );
      onExport(selectedShows, response.exports);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(`Export failed: ${err.statusText}`);
      } else {
        setError('Export failed');
      }
      setIsExporting(false);
    }
  };

  useInput((input, key) => {
    if (isLoading || isExporting) return;

    if (input === 'q') {
      exit();
      return;
    }

    if (key.escape) {
      onBack();
      return;
    }

    if (input === 't') {
      onChangeTarget();
      return;
    }

    if (key.downArrow) {
      setCursor(prev => Math.min(prev + 1, shows.length - 1));
    }

    if (key.upArrow) {
      setCursor(prev => Math.max(prev - 1, 0));
    }

    if (input === ' ') {
      const show = shows[cursor];
      if (show) {
        setSelected(prev => {
          const next = new Set(prev);
          if (next.has(show.id)) {
            next.delete(show.id);
          } else {
            next.add(show.id);
          }
          return next;
        });
      }
    }

    if (key.return) {
      handleExport();
    }

    // Pagination
    if (input === 'n' && offset + limit < total) {
      setOffset(prev => prev + limit);
      setCursor(0);
    }

    if (input === 'p' && offset > 0) {
      setOffset(prev => Math.max(prev - limit, 0));
      setCursor(0);
    }

    // Status filter
    if (input === 'a') {
      setStatusFilter('approved');
      setOffset(0);
      setCursor(0);
    }
    if (input === 'd') {
      setStatusFilter('pending');
      setOffset(0);
      setCursor(0);
    }
    if (input === 'r') {
      setStatusFilter('rejected');
      setOffset(0);
      setCursor(0);
    }
    if (input === 'v') {
      setStatusFilter('private');
      setOffset(0);
      setCursor(0);
    }
    if (input === 'c') {
      setStatusFilter('');
      setOffset(0);
      setCursor(0);
    }

    // Select all on current page
    if (input === 's') {
      const allSelected = shows.every(s => selected.has(s.id));
      if (allSelected) {
        // Deselect all
        setSelected(prev => {
          const next = new Set(prev);
          shows.forEach(s => next.delete(s.id));
          return next;
        });
      } else {
        // Select all
        setSelected(prev => {
          const next = new Set(prev);
          shows.forEach(s => next.add(s.id));
          return next;
        });
      }
    }
  });

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'approved':
        return 'green';
      case 'pending':
        return 'yellow';
      case 'rejected':
        return 'red';
      case 'private':
        return 'blue';
      default:
        return 'white';
    }
  };

  if (isLoading) {
    return (
      <Box flexDirection="column" padding={1}>
        <Text>
          <Text color="green">
            <Spinner type="dots" />
          </Text>
          {' '}Loading shows...
        </Text>
      </Box>
    );
  }

  if (isExporting) {
    return (
      <Box flexDirection="column" padding={1}>
        <Text>
          <Text color="green">
            <Spinner type="dots" />
          </Text>
          {' '}Exporting {selected.size} show(s)...
        </Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column" padding={1}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text>
          <Text bold color="cyan">Source:</Text> {sourceEnvName}
          {'  '}
          <Text bold color="magenta">Target:</Text> {targetEnvName}
          {'  '}
          <Text bold color="yellow">Selected:</Text> {selected.size}
        </Text>
      </Box>

      {/* Filter bar */}
      <Box marginBottom={1}>
        <Text dimColor>
          Filter: [{statusFilter || 'all'}] | a:approved d:pending r:rejected v:private c:clear
        </Text>
      </Box>

      {error && (
        <Box marginBottom={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      {/* Show list */}
      <Box flexDirection="column" marginBottom={1}>
        {shows.length === 0 ? (
          <Text dimColor>No shows found</Text>
        ) : (
          shows.map((show, index) => {
            const isSelected = selected.has(show.id);
            const isCursor = index === cursor;
            const headliner = show.artists.find(a => a.is_headliner)?.name || show.artists[0]?.name || 'Unknown';
            const venue = show.venues[0]?.name || 'Unknown Venue';

            return (
              <Box key={show.id}>
                <Text color={isCursor ? 'yellow' : 'white'}>
                  {isCursor ? '>' : ' '}
                  [{isSelected ? 'x' : ' '}] {formatDate(show.event_date)}
                  <Text color={getStatusColor(show.status)}>[{show.status.slice(0, 3)}]</Text>
                  {' '}{headliner} @ {venue}
                  {show.city && <Text dimColor> ({show.city})</Text>}
                </Text>
              </Box>
            );
          })
        )}
      </Box>

      {/* Pagination info */}
      <Box marginBottom={1}>
        <Text dimColor>
          Showing {offset + 1}-{Math.min(offset + shows.length, total)} of {total}
          {offset > 0 && ' | p:prev'}
          {offset + limit < total && ' | n:next'}
        </Text>
      </Box>

      {/* Controls */}
      <Box>
        <Text dimColor>
          Space:toggle s:select-all Enter:export t:target Esc:back q:quit
        </Text>
      </Box>
    </Box>
  );
}
