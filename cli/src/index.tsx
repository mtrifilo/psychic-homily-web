#!/usr/bin/env node
import React, { useState, useCallback } from 'react';
import { render, Box, Text } from 'ink';
import { EnvironmentSelectScreen } from './screens/environment-select.js';
import { LoginScreen } from './screens/login.js';
import { ShowListScreen } from './screens/show-list.js';
import { ExportPreviewScreen } from './screens/export-preview.js';
import { ImportResultScreen } from './screens/import-result.js';
import { Environment } from './config/environments.js';
import { ApiClient, Show } from './api/client.js';

type Screen =
  | 'select-source'
  | 'login-source'
  | 'select-target'
  | 'login-target'
  | 'show-list'
  | 'export-preview'
  | 'import-result';

interface ImportResultData {
  results: {
    success: boolean;
    show?: Show;
    error?: string;
  }[];
  successCount: number;
  errorCount: number;
}

function App() {
  const [screen, setScreen] = useState<Screen>('select-source');
  const [sourceEnv, setSourceEnv] = useState<Environment | null>(null);
  const [targetEnv, setTargetEnv] = useState<Environment | null>(null);
  const [sourceClient, setSourceClient] = useState<ApiClient | null>(null);
  const [targetClient, setTargetClient] = useState<ApiClient | null>(null);
  const [selectedShows, setSelectedShows] = useState<Show[]>([]);
  const [exportedData, setExportedData] = useState<string[]>([]);
  const [importResult, setImportResult] = useState<ImportResultData | null>(null);

  // Source environment selection
  const handleSourceSelect = useCallback((env: Environment) => {
    setSourceEnv(env);
    const client = new ApiClient(env.apiUrl, env.key);
    setSourceClient(client);
    setScreen('login-source');
  }, []);

  // Source login success
  const handleSourceLoginSuccess = useCallback(() => {
    setScreen('select-target');
  }, []);

  // Target environment selection
  const handleTargetSelect = useCallback((env: Environment) => {
    setTargetEnv(env);
    const client = new ApiClient(env.apiUrl, env.key);
    setTargetClient(client);
    setScreen('login-target');
  }, []);

  // Target login success
  const handleTargetLoginSuccess = useCallback(() => {
    setScreen('show-list');
  }, []);

  // Export shows
  const handleExport = useCallback((shows: Show[], data: string[]) => {
    setSelectedShows(shows);
    setExportedData(data);
    setScreen('export-preview');
  }, []);

  // Import confirmed
  const handleImportConfirm = useCallback(async () => {
    if (!targetClient) return;

    try {
      const result = await targetClient.bulkImportConfirm(exportedData);
      setImportResult({
        results: result.results,
        successCount: result.success_count,
        errorCount: result.error_count,
      });
      setScreen('import-result');
    } catch {
      // Error handling is done in the preview screen
    }
  }, [targetClient, exportedData]);

  // Change target from show list
  const handleChangeTarget = useCallback(() => {
    setScreen('select-target');
  }, []);

  // Back to show list from preview
  const handleBackToShowList = useCallback(() => {
    setSelectedShows([]);
    setExportedData([]);
    setScreen('show-list');
  }, []);

  // Done with import
  const handleImportDone = useCallback(() => {
    setSelectedShows([]);
    setExportedData([]);
    setImportResult(null);
    setScreen('show-list');
  }, []);

  // Back to source selection
  const handleBackToSourceSelect = useCallback(() => {
    setSourceEnv(null);
    setSourceClient(null);
    setScreen('select-source');
  }, []);

  // Back to target selection
  const handleBackToTargetSelect = useCallback(() => {
    setTargetEnv(null);
    setTargetClient(null);
    setScreen('select-target');
  }, []);

  switch (screen) {
    case 'select-source':
      return (
        <EnvironmentSelectScreen
          title="Select Source Environment:"
          onSelect={handleSourceSelect}
        />
      );

    case 'login-source':
      if (!sourceClient || !sourceEnv) {
        return <Text color="red">Error: No source environment selected</Text>;
      }
      return (
        <LoginScreen
          client={sourceClient}
          apiUrl={sourceEnv.apiUrl}
          environmentName={sourceEnv.name}
          environmentKey={sourceEnv.key}
          onSuccess={handleSourceLoginSuccess}
          onBack={handleBackToSourceSelect}
        />
      );

    case 'select-target':
      return (
        <EnvironmentSelectScreen
          title="Select Target Environment:"
          onSelect={handleTargetSelect}
          currentSelection={sourceEnv?.key}
        />
      );

    case 'login-target':
      if (!targetClient || !targetEnv) {
        return <Text color="red">Error: No target environment selected</Text>;
      }
      return (
        <LoginScreen
          client={targetClient}
          apiUrl={targetEnv.apiUrl}
          environmentName={targetEnv.name}
          environmentKey={targetEnv.key}
          onSuccess={handleTargetLoginSuccess}
          onBack={handleBackToTargetSelect}
        />
      );

    case 'show-list':
      if (!sourceClient || !sourceEnv || !targetEnv) {
        return <Text color="red">Error: Missing environment configuration</Text>;
      }
      return (
        <ShowListScreen
          sourceClient={sourceClient}
          sourceEnvName={sourceEnv.name}
          targetEnvName={targetEnv.name}
          onExport={handleExport}
          onChangeTarget={handleChangeTarget}
          onBack={handleBackToTargetSelect}
        />
      );

    case 'export-preview':
      if (!targetClient || !targetEnv) {
        return <Text color="red">Error: No target environment selected</Text>;
      }
      return (
        <ExportPreviewScreen
          targetClient={targetClient}
          targetEnvName={targetEnv.name}
          selectedShows={selectedShows}
          exportedData={exportedData}
          onConfirm={handleImportConfirm}
          onBack={handleBackToShowList}
        />
      );

    case 'import-result':
      if (!targetEnv || !importResult) {
        return <Text color="red">Error: No import result</Text>;
      }
      return (
        <ImportResultScreen
          targetEnvName={targetEnv.name}
          results={importResult.results}
          successCount={importResult.successCount}
          errorCount={importResult.errorCount}
          onDone={handleImportDone}
        />
      );

    default:
      return (
        <Box>
          <Text color="red">Unknown screen</Text>
        </Box>
      );
  }
}

// Main entry point
render(<App />);
