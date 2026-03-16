import type { EnvironmentConfig } from "./types";

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

  constructor(env: EnvironmentConfig) {
    // Strip trailing slash
    this.baseUrl = env.url.replace(/\/+$/, "");
    this.token = env.token;
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

    const response = await fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: AbortSignal.timeout(30_000),
    });

    const text = await response.text();

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
