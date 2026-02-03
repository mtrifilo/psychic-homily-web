import React, { useState, useEffect } from 'react';
import { Box, Text, useInput, useApp } from 'ink';
import Spinner from 'ink-spinner';
import { ApiClient, Show, ImportPreview, ApiError } from '../api/client.js';

interface ExportPreviewScreenProps {
  targetClient: ApiClient;
  targetEnvName: string;
  selectedShows: Show[];
  exportedData: string[];
  onConfirm: () => void;
  onBack: () => void;
}

export function ExportPreviewScreen({
  targetClient,
  targetEnvName,
  selectedShows,
  exportedData,
  onConfirm,
  onBack,
}: ExportPreviewScreenProps) {
  const { exit } = useApp();
  const [previews, setPreviews] = useState<ImportPreview[]>([]);
  const [summary, setSummary] = useState<{
    new_artists: number;
    new_venues: number;
    warning_count: number;
    can_import_all: boolean;
  } | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isImporting, setIsImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [scrollOffset, setScrollOffset] = useState(0);
  const maxVisibleShows = 5;

  useEffect(() => {
    const fetchPreview = async () => {
      setIsLoading(true);
      setError(null);

      try {
        const response = await targetClient.bulkImportPreview(exportedData);
        setPreviews(response.previews);
        setSummary(response.summary);
      } catch (err) {
        if (err instanceof ApiError) {
          setError(`Preview failed: ${err.statusText}`);
        } else {
          setError('Failed to preview import');
        }
      } finally {
        setIsLoading(false);
      }
    };

    fetchPreview();
  }, [targetClient, exportedData]);

  const handleConfirm = async () => {
    setIsImporting(true);
    setError(null);

    try {
      await targetClient.bulkImportConfirm(exportedData);
      onConfirm();
    } catch (err) {
      if (err instanceof ApiError) {
        setError(`Import failed: ${err.statusText}`);
      } else {
        setError('Import failed');
      }
      setIsImporting(false);
    }
  };

  useInput((input, key) => {
    if (isLoading || isImporting) return;

    if (input === 'q') {
      exit();
      return;
    }

    if (key.escape) {
      onBack();
      return;
    }

    if (key.return && summary?.can_import_all) {
      handleConfirm();
      return;
    }

    if (key.downArrow && scrollOffset + maxVisibleShows < previews.length) {
      setScrollOffset(prev => prev + 1);
    }

    if (key.upArrow && scrollOffset > 0) {
      setScrollOffset(prev => prev - 1);
    }
  });

  if (isLoading) {
    return (
      <Box flexDirection="column" padding={1}>
        <Text>
          <Text color="green">
            <Spinner type="dots" />
          </Text>
          {' '}Analyzing import to {targetEnvName}...
        </Text>
      </Box>
    );
  }

  if (isImporting) {
    return (
      <Box flexDirection="column" padding={1}>
        <Text>
          <Text color="green">
            <Spinner type="dots" />
          </Text>
          {' '}Importing {selectedShows.length} show(s) to {targetEnvName}...
        </Text>
      </Box>
    );
  }

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  };

  const visiblePreviews = previews.slice(scrollOffset, scrollOffset + maxVisibleShows);

  return (
    <Box flexDirection="column" padding={1}>
      {/* Header */}
      <Box marginBottom={1}>
        <Text bold color="cyan">
          Export Preview: {selectedShows.length} shows → {targetEnvName}
        </Text>
      </Box>

      {error && (
        <Box marginBottom={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      {/* Show previews */}
      <Box flexDirection="column" marginBottom={1}>
        {visiblePreviews.map((preview, idx) => {
          const show = selectedShows[scrollOffset + idx];
          const headliner = show?.artists.find(a => a.is_headliner)?.name || show?.artists[0]?.name;
          const venue = show?.venues[0]?.name;

          return (
            <Box key={idx} flexDirection="column" marginBottom={1}>
              <Text bold>
                Show {scrollOffset + idx + 1}: {headliner} @ {venue} ({formatDate(preview.show.event_date)})
              </Text>

              {/* Artists */}
              <Box marginLeft={2}>
                <Text>
                  Artists:{' '}
                  {preview.artists.map((a, i) => (
                    <Text key={i}>
                      {i > 0 ? ', ' : ''}
                      {a.name}
                      {a.will_create ? (
                        <Text color="yellow"> (new)</Text>
                      ) : (
                        <Text color="green"> (exists)</Text>
                      )}
                    </Text>
                  ))}
                </Text>
              </Box>

              {/* Venues */}
              <Box marginLeft={2}>
                <Text>
                  Venue:{' '}
                  {preview.venues.map((v, i) => (
                    <Text key={i}>
                      {i > 0 ? ', ' : ''}
                      {v.name}
                      {v.will_create ? (
                        <Text color="yellow"> (new)</Text>
                      ) : (
                        <Text color="green"> (exists)</Text>
                      )}
                    </Text>
                  ))}
                </Text>
              </Box>

              {/* Warnings */}
              {preview.warnings.length > 0 && (
                <Box marginLeft={2} flexDirection="column">
                  {preview.warnings.map((warning, i) => (
                    <Text key={i} color="yellow">
                      ⚠ {warning}
                    </Text>
                  ))}
                </Box>
              )}

              {/* Status */}
              <Box marginLeft={2}>
                {preview.can_import ? (
                  <Text color="green">✓ Ready</Text>
                ) : (
                  <Text color="red">✗ Cannot import</Text>
                )}
              </Box>
            </Box>
          );
        })}
      </Box>

      {/* Scroll indicator */}
      {previews.length > maxVisibleShows && (
        <Box marginBottom={1}>
          <Text dimColor>
            Showing {scrollOffset + 1}-{Math.min(scrollOffset + maxVisibleShows, previews.length)} of {previews.length}
            {' (use arrows to scroll)'}
          </Text>
        </Box>
      )}

      {/* Summary */}
      {summary && (
        <Box marginBottom={1} flexDirection="column">
          <Text bold>Summary:</Text>
          <Text>
            {summary.new_artists} new artist(s), {summary.new_venues} new venue(s)
            {summary.warning_count > 0 && (
              <Text color="yellow">, {summary.warning_count} warning(s)</Text>
            )}
          </Text>
        </Box>
      )}

      {/* Controls */}
      <Box>
        {summary?.can_import_all ? (
          <Text>
            <Text color="green">[Enter]</Text> Confirm
            {'  '}
            <Text dimColor>[Esc] Cancel  [q] Quit</Text>
          </Text>
        ) : (
          <Text>
            <Text color="red">Cannot import - fix errors first</Text>
            {'  '}
            <Text dimColor>[Esc] Cancel  [q] Quit</Text>
          </Text>
        )}
      </Box>
    </Box>
  );
}
