/** Environment configuration for API connectivity. */
export interface EnvironmentConfig {
  url: string;
  token: string;
  /** Runtime-only flag for verbose request/response logging. Not persisted. */
  verbose?: boolean;
}

/** Top-level configuration stored at ~/.psychic-homily/config.json */
export interface PHConfig {
  environments: Record<string, EnvironmentConfig>;
  default_environment: string;
}

/** Global CLI options available on all commands. */
export interface GlobalOptions {
  env?: string;
  confirm?: boolean;
}

/** Standardized API error response from the backend. */
export interface APIErrorResponse {
  success: false;
  message: string;
  error_code?: string;
  request_id?: string;
}

/** Standardized API success response wrapper. */
export interface APIResponse<T> {
  data: T;
  status: number;
}

/** Entity types supported by the CLI. */
export type EntityType =
  | "artist"
  | "venue"
  | "show"
  | "release"
  | "label"
  | "festival";
