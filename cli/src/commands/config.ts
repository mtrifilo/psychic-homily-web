import { readConfig, writeConfig, getConfigPath } from "../lib/config";
import * as display from "../lib/display";
import { bold, gray, green, dim } from "../lib/ansi";

export async function runConfigShow(): Promise<void> {
  const config = await readConfig();
  const configPath = getConfigPath();

  display.header("PH CLI Configuration");
  display.kv("Config file", configPath);
  display.kv("Default env", config.default_environment || "(not set)");

  const envNames = Object.keys(config.environments);

  if (envNames.length === 0) {
    process.stderr.write(
      `\n  ${dim("No environments configured. Run")} ${bold("ph init")} ${dim("to set one up.")}\n`,
    );
    return;
  }

  process.stderr.write(`\n  ${bold("Environments:")}\n`);
  for (const name of envNames) {
    const env = config.environments[name];
    const isDefault = name === config.default_environment;
    const marker = isDefault ? green(" (default)") : "";
    const tokenPreview = env.token
      ? `${env.token.slice(0, 8)}...${env.token.slice(-4)}`
      : "(no token)";
    process.stderr.write(
      `    ${bold(name)}${marker}\n` +
        `      ${gray("url:")}   ${env.url}\n` +
        `      ${gray("token:")} ${tokenPreview}\n`,
    );
  }
  process.stderr.write("\n");
}

export async function runConfigSet(key: string, value: string): Promise<void> {
  const config = await readConfig();

  if (key === "default_environment" || key === "default") {
    if (!config.environments[value]) {
      display.error(
        `Environment "${value}" does not exist. Available: ${Object.keys(config.environments).join(", ") || "(none)"}`,
      );
      process.exit(1);
    }
    config.default_environment = value;
    await writeConfig(config);
    display.success(`Default environment set to "${value}"`);
    return;
  }

  display.error(
    `Unknown config key "${key}". Supported keys: default_environment`,
  );
  process.exit(1);
}
