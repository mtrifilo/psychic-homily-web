import type { EnvironmentConfig } from "./types";
import { dim, gray, cyan, yellow } from "./ansi";

export class APIError extends Error {
  constructor(
    public status: number,
    public errorCode: string | undefined,
    message: string,
    public requestId?: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

/** API client for the Psychic Homily backend. */
export class APIClient {
  private baseUrl: string;
  private token: string;
  private verbose: boolean;

  constructor(env: EnvironmentConfig) {
    // Strip trailing slash
    this.baseUrl = env.url.replace(/\/+$/, "");
    this.token = env.token;
    this.verbose = env.verbose ?? false;
  }

  /** Make an authenticated GET request. */
  async get<T>(path: string, params?: Record<string, string>): Promise<T> {
    const url = this.buildUrl(path, params);
    return this.request<T>("GET", url);
  }

  /** Make an authenticated POST request. */
  async post<T>(path: string, body?: unknown): Promise<T> {
    const url = this.buildUrl(path);
    return this.request<T>("POST", url, body);
  }

  /** Make an authenticated PUT request. */
  async put<T>(path: string, body?: unknown): Promise<T> {
    const url = this.buildUrl(path);
    return this.request<T>("PUT", url, body);
  }

  /** Make an authenticated PATCH request. */
  async patch<T>(path: string, body?: unknown): Promise<T> {
    const url = this.buildUrl(path);
    return this.request<T>("PATCH", url, body);
  }

  /** Make an authenticated DELETE request. */
  async delete<T>(path: string): Promise<T> {
    const url = this.buildUrl(path);
    return this.request<T>("DELETE", url);
  }

  /** Test the API connection by hitting the health endpoint. */
  async healthCheck(): Promise<boolean> {
    try {
      const url = this.buildUrl("/health");
      const response = await fetch(url, {
        method: "GET",
        headers: { "User-Agent": "ph-cli/0.1.0" },
        signal: AbortSignal.timeout(10_000),
      });
      return response.ok;
    } catch {
      return false;
    }
  }

  /** Verify the token is valid by fetching the auth profile. */
  async verifyAuth(): Promise<{ id: number; username: string; is_admin: boolean } | null> {
    try {
      const result = await this.get<{
        success: boolean;
        user: { id: number; username: string; is_admin: boolean };
      }>("/auth/profile");
      if (result.success && result.user) {
        return result.user;
      }
      return null;
    } catch {
      return null;
    }
  }

  private buildUrl(path: string, params?: Record<string, string>): string {
    const url = new URL(path, this.baseUrl);
    if (params) {
      for (const [key, value] of Object.entries(params)) {
        if (value) url.searchParams.set(key, value);
      }
    }
    return url.toString();
  }

  private logVerbose(text: string): void {
    process.stderr.write(text);
  }

  private logRequest(method: string, url: string, headers: Record<string, string>, body?: unknown): void {
    if (!this.verbose) return;

    this.logVerbose(`\n${dim("───── Request ─────")}\n`);
    this.logVerbose(`${cyan(method)} ${url}\n`);

    this.logVerbose(`${gray("Headers:")}\n`);
    for (const [key, value] of Object.entries(headers)) {
      const displayValue = key === "Authorization" ? `Bearer ${this.token.slice(0, 8)}...` : value;
      this.logVerbose(`  ${dim(key + ":")} ${displayValue}\n`);
    }

    if (body !== undefined) {
      this.logVerbose(`${gray("Body:")}\n`);
      try {
        this.logVerbose(`${JSON.stringify(body, null, 2)}\n`);
      } catch {
        this.logVerbose(`  ${dim("(unable to serialize body)")}\n`);
      }
    }
  }

  private logResponse(status: number, statusText: string, headers: Headers, body: string): void {
    if (!this.verbose) return;

    this.logVerbose(`\n${dim("───── Response ─────")}\n`);

    const statusColor = status >= 400 ? yellow : cyan;
    this.logVerbose(`${statusColor(`${status} ${statusText}`)}\n`);

    this.logVerbose(`${gray("Headers:")}\n`);
    headers.forEach((value, key) => {
      this.logVerbose(`  ${dim(key + ":")} ${value}\n`);
    });

    if (body) {
      this.logVerbose(`${gray("Body:")}\n`);
      try {
        const parsed = JSON.parse(body);
        this.logVerbose(`${JSON.stringify(parsed, null, 2)}\n`);
      } catch {
        // Not JSON — print raw (truncated if very long)
        const maxLen = 2000;
        const truncated = body.length > maxLen ? body.slice(0, maxLen) + `\n${dim(`... (${body.length - maxLen} more bytes)`)}` : body;
        this.logVerbose(`${truncated}\n`);
      }
    }

    this.logVerbose(`${dim("────────────────────")}\n`);
  }

  private async request<T>(
    method: string,
    url: string,
    body?: unknown,
  ): Promise<T> {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.token}`,
      "User-Agent": "ph-cli/0.1.0",
    };

    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
    }

    this.logRequest(method, url, headers, body);

    const response = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: AbortSignal.timeout(30_000),
    });

    const text = await response.text();

    this.logResponse(response.status, response.statusText, response.headers, text);

    if (!response.ok) {
      let message = `HTTP ${response.status}: ${response.statusText}`;
      let errorCode: string | undefined;
      let requestId: string | undefined;

      try {
        const parsed = JSON.parse(text);
        if (parsed.message) message = parsed.message;
        errorCode = parsed.error_code;
        requestId = parsed.request_id;
      } catch {
        // Use the raw status text
      }

      throw new APIError(response.status, errorCode, message, requestId);
    }

    if (!text) return {} as T;

    try {
      return JSON.parse(text) as T;
    } catch {
      return {} as T;
    }
  }
}
