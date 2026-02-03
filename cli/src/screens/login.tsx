import React, { useState, useEffect, useCallback } from 'react';
import { Box, Text, useInput, useApp } from 'ink';
import TextInput from 'ink-text-input';
import Spinner from 'ink-spinner';
import { ApiClient, ApiError } from '../api/client.js';
import { getCredentialsFromEnv, storeToken } from '../config/auth.js';

interface LoginScreenProps {
  client: ApiClient;
  apiUrl: string;
  environmentName: string;
  environmentKey: string;
  onSuccess: () => void;
  onBack: () => void;
}

type LoginMode = 'select' | 'token' | 'manual';
type InputField = 'email' | 'password';

export function LoginScreen({ client, apiUrl, environmentName, environmentKey, onSuccess, onBack }: LoginScreenProps) {
  const { exit } = useApp();
  const [mode, setMode] = useState<LoginMode>('select');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [token, setToken] = useState('');
  const [activeField, setActiveField] = useState<InputField>('email');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [statusMessage, setStatusMessage] = useState('Checking authentication...');

  const verifyAdmin = useCallback(async (): Promise<boolean> => {
    try {
      const profile = await client.getProfile();
      if (profile.user?.is_admin) {
        onSuccess();
        return true;
      } else {
        setError('Account is not an admin');
        return false;
      }
    } catch {
      setError('Failed to verify admin status');
      return false;
    }
  }, [client, onSuccess]);

  const performManualLogin = useCallback(async (loginEmail: string, loginPassword: string, isAutoLogin: boolean) => {
    setIsLoading(true);
    setError(null);
    if (isAutoLogin) {
      setStatusMessage('Auto-login with env credentials...');
    } else {
      setStatusMessage('Logging in...');
    }

    try {
      const response = await client.login(loginEmail, loginPassword);
      if (!response.user.is_admin) {
        setError('Account is not an admin');
        setIsLoading(false);
        return false;
      }
      onSuccess();
      return true;
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          setError(isAutoLogin ? 'Env credentials invalid' : 'Invalid email or password');
        } else {
          setError(`Login failed: ${err.statusText}`);
        }
      } else {
        setError('Connection failed. Is the server running?');
      }
      setIsLoading(false);
      return false;
    }
  }, [client, onSuccess]);

  const performTokenLogin = useCallback(async (pastedToken: string) => {
    setIsLoading(true);
    setError(null);
    setStatusMessage('Verifying token...');

    // Store the token (24 hour expiry)
    storeToken(environmentKey, pastedToken, 86400);

    // Verify the token works and user is admin
    const isAdmin = await verifyAdmin();
    if (!isAdmin) {
      setIsLoading(false);
      setMode('select');
    }
  }, [environmentKey, verifyAdmin]);

  // Check if already authenticated or can auto-login
  useEffect(() => {
    const checkAuth = async () => {
      // First check existing token
      if (client.isAuthenticated()) {
        setStatusMessage('Verifying existing token...');
        try {
          const profile = await client.getProfile();
          if (profile.user?.is_admin) {
            onSuccess();
            return;
          } else {
            setError('Account is not an admin');
          }
        } catch {
          // Token might be invalid, continue to login
        }
      }

      // Check for environment variable credentials
      const envCreds = getCredentialsFromEnv(environmentKey);
      if (envCreds) {
        setStatusMessage('Found env credentials, logging in...');
        const success = await performManualLogin(envCreds.email, envCreds.password, true);
        if (success) return;
      }

      setCheckingAuth(false);
    };
    checkAuth();
  }, [client, environmentKey, onSuccess, performManualLogin]);

  useInput((input, key) => {
    if (isLoading || checkingAuth) return;

    if (key.escape) {
      if (mode === 'manual' || mode === 'token') {
        setMode('select');
        setError(null);
        setToken('');
      } else {
        onBack();
      }
      return;
    }

    if (input === 'q' && mode === 'select') {
      exit();
      return;
    }

    // Mode selection
    if (mode === 'select') {
      if (input === 't' || input === '1') {
        setMode('token');
        setError(null);
        return;
      }
      if (input === 'm' || input === '2') {
        setMode('manual');
        setError(null);
        return;
      }
    }

    // Manual login mode
    if (mode === 'manual') {
      if (input === 'q' && !email && !password) {
        setMode('select');
        return;
      }

      if (key.tab || key.downArrow) {
        setActiveField(activeField === 'email' ? 'password' : 'email');
      }

      if (key.upArrow) {
        setActiveField(activeField === 'password' ? 'email' : 'password');
      }
    }
  });

  const handleManualSubmit = async () => {
    if (!email || !password) {
      setError('Email and password are required');
      return;
    }
    await performManualLogin(email, password, false);
  };

  // Loading state
  if (checkingAuth || isLoading) {
    return (
      <Box flexDirection="column" padding={1}>
        <Box marginBottom={1}>
          <Text bold color="cyan">Login to {environmentName}</Text>
        </Box>
        <Text>
          <Text color="green">
            <Spinner type="dots" />
          </Text>
          {' '}{statusMessage}
        </Text>
        {error && (
          <Box marginTop={1}>
            <Text color="red">{error}</Text>
          </Box>
        )}
      </Box>
    );
  }

  // Login method selection
  if (mode === 'select') {
    return (
      <Box flexDirection="column" padding={1}>
        <Box marginBottom={1}>
          <Text bold color="cyan">Login to {environmentName}</Text>
        </Box>

        {error && (
          <Box marginBottom={1}>
            <Text color="red">{error}</Text>
          </Box>
        )}

        <Box marginBottom={1}>
          <Text>Choose login method:</Text>
        </Box>

        <Box flexDirection="column" marginBottom={1}>
          <Text>
            <Text color="yellow">[t]</Text> Paste token from web UI <Text dimColor>(recommended)</Text>
          </Text>
          <Text>
            <Text color="yellow">[m]</Text> Login with email/password
          </Text>
        </Box>

        <Box marginTop={1}>
          <Text dimColor>Get token: Log into web UI → Settings → Generate CLI Token</Text>
        </Box>

        <Box marginTop={1}>
          <Text dimColor>Press t or m to select, Esc to go back, q to quit</Text>
        </Box>
      </Box>
    );
  }

  // Token paste mode
  if (mode === 'token') {
    return (
      <Box flexDirection="column" padding={1}>
        <Box marginBottom={1}>
          <Text bold color="cyan">Login to {environmentName}</Text>
        </Box>

        {error && (
          <Box marginBottom={1}>
            <Text color="red">{error}</Text>
          </Box>
        )}

        <Box marginBottom={1}>
          <Text>Paste your CLI token from the web UI:</Text>
        </Box>

        <Box marginBottom={1}>
          <Text color="yellow">&gt; </Text>
          <TextInput
            value={token}
            onChange={setToken}
            onSubmit={() => {
              if (token.trim()) {
                performTokenLogin(token.trim());
              }
            }}
            placeholder="eyJ..."
          />
        </Box>

        <Box flexDirection="column">
          <Text dimColor>Paste token and press Enter. Esc to go back.</Text>
        </Box>
      </Box>
    );
  }

  // Manual email/password login
  return (
    <Box flexDirection="column" padding={1}>
      <Box marginBottom={1}>
        <Text bold color="cyan">Login to {environmentName}</Text>
      </Box>

      {error && (
        <Box marginBottom={1}>
          <Text color="red">{error}</Text>
        </Box>
      )}

      <Box flexDirection="column" marginBottom={1}>
        <Box>
          <Text color={activeField === 'email' ? 'yellow' : 'white'}>
            {activeField === 'email' ? '>' : ' '} Email:{' '}
          </Text>
          {activeField === 'email' ? (
            <TextInput
              value={email}
              onChange={setEmail}
              onSubmit={() => setActiveField('password')}
            />
          ) : (
            <Text>{email || '(empty)'}</Text>
          )}
        </Box>

        <Box>
          <Text color={activeField === 'password' ? 'yellow' : 'white'}>
            {activeField === 'password' ? '>' : ' '} Password:{' '}
          </Text>
          {activeField === 'password' ? (
            <TextInput
              value={password}
              onChange={setPassword}
              onSubmit={handleManualSubmit}
              mask="*"
            />
          ) : (
            <Text>{password ? '*'.repeat(password.length) : '(empty)'}</Text>
          )}
        </Box>
      </Box>

      <Box flexDirection="column">
        <Text dimColor>Tab/Arrow: Switch field  Enter: Submit  Esc: Back</Text>
      </Box>
    </Box>
  );
}
