import fs from 'fs';
import path from 'path';
import os from 'os';

export interface StoredToken {
  token: string;
  expiresAt: number; // Unix timestamp in milliseconds
}

export interface AuthStorage {
  [environmentKey: string]: StoredToken;
}

export interface EnvironmentCredentials {
  email: string;
  password: string;
}

const CONFIG_DIR = path.join(os.homedir(), '.config', 'psychic-homily');
const AUTH_FILE = path.join(CONFIG_DIR, 'auth.json');

// Environment variable names for credentials
// PH_LOCAL_EMAIL, PH_LOCAL_PASSWORD
// PH_STAGE_EMAIL, PH_STAGE_PASSWORD
// PH_PRODUCTION_EMAIL, PH_PRODUCTION_PASSWORD
export function getCredentialsFromEnv(environmentKey: string): EnvironmentCredentials | null {
  const prefix = `PH_${environmentKey.toUpperCase()}`;
  const email = process.env[`${prefix}_EMAIL`];
  const password = process.env[`${prefix}_PASSWORD`];

  if (email && password) {
    return { email, password };
  }
  return null;
}

function ensureConfigDir(): void {
  if (!fs.existsSync(CONFIG_DIR)) {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }
}

function readAuthStorage(): AuthStorage {
  try {
    if (fs.existsSync(AUTH_FILE)) {
      const content = fs.readFileSync(AUTH_FILE, 'utf-8');
      return JSON.parse(content);
    }
  } catch {
    // Ignore errors, return empty storage
  }
  return {};
}

function writeAuthStorage(storage: AuthStorage): void {
  ensureConfigDir();
  fs.writeFileSync(AUTH_FILE, JSON.stringify(storage, null, 2));
}

export function getStoredToken(environmentKey: string): StoredToken | null {
  const storage = readAuthStorage();
  const stored = storage[environmentKey];

  if (!stored) {
    return null;
  }

  // Check if token is expired (with 5 minute buffer)
  const now = Date.now();
  const buffer = 5 * 60 * 1000; // 5 minutes
  if (stored.expiresAt - buffer < now) {
    // Token is expired or about to expire
    return null;
  }

  return stored;
}

export function storeToken(environmentKey: string, token: string, expiresIn: number): void {
  const storage = readAuthStorage();
  storage[environmentKey] = {
    token,
    expiresAt: Date.now() + expiresIn * 1000,
  };
  writeAuthStorage(storage);
}

export function clearToken(environmentKey: string): void {
  const storage = readAuthStorage();
  delete storage[environmentKey];
  writeAuthStorage(storage);
}

export function clearAllTokens(): void {
  writeAuthStorage({});
}
