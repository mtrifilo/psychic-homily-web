import { readConfig, writeConfig, getConfigPath } from "../lib/config";
import { APIClient } from "../lib/api";
import * as display from "../lib/display";
import type { PHConfig } from "../lib/types";

interface InitOptions {
  url?: string;
  token?: string;
  name?: string;
  setDefault?: boolean;
}

export async function runInit(options: InitOptions): Promise<void> {
  const envName = options.name || "production";
  const url = options.url;
  const token = options.token;

  if (!url || !token) {
    display.error(
      "Both --url and --token are required.\n\n" +
        "  Usage:\n" +
        '    ph init --url https://api.psychichomily.com --token phk_xxx\n' +
        '    ph init --url http://localhost:8080 --token phk_xxx --name local\n',
    );
    process.exit(1);
  }

  // Validate token format
  if (!token.startsWith("phk_")) {
    display.warn(
      'Token does not start with "phk_". PH API tokens use the phk_ prefix.',
    );
  }

  display.info(`Testing connection to ${url}...`);

  const client = new APIClient({ url, token });

  // Test health endpoint
  const healthy = await client.healthCheck();
  if (!healthy) {
    display.error(`Could not reach ${url}/health. Is the API running?`);
    process.exit(1);
  }
  display.success("API is reachable");

  // Verify auth token
  const user = await client.verifyAuth();
  if (!user) {
    display.error("Token is invalid or expired.");
    process.exit(1);
  }
  display.success(`Authenticated as ${user.username} (ID: ${user.id})`);

  if (!user.is_admin) {
    display.warn(
      "This user is not an admin. Most ph commands require admin access.",
    );
  }

  // Save config
  const config = await readConfig();
  config.environments[envName] = { url, token };

  // Set as default if it's the first environment or --set-default is passed
  if (
    Object.keys(config.environments).length === 1 ||
    options.setDefault
  ) {
    config.default_environment = envName;
  }

  await writeConfig(config);

  display.success(`Environment "${envName}" saved to ${getConfigPath()}`);
  if (config.default_environment === envName) {
    display.info(`"${envName}" is the default environment`);
  }
}
