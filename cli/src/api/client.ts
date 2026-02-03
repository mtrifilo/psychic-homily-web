import { getStoredToken, storeToken } from '../config/auth.js';

export interface Show {
  id: number;
  slug: string;
  title: string;
  event_date: string;
  city: string | null;
  state: string | null;
  status: string;
  venues: Venue[];
  artists: Artist[];
}

export interface Venue {
  id: number;
  slug: string;
  name: string;
  city: string;
  state: string;
  verified: boolean;
}

export interface Artist {
  id: number;
  slug: string;
  name: string;
  is_headliner: boolean | null;
}

export interface ShowsResponse {
  shows: Show[];
  total: number;
}

export interface LoginResponse {
  success: boolean;
  message: string;
  user: {
    id: number;
    email: string;
    is_admin: boolean;
  };
}

export interface ProfileResponse {
  success: boolean;
  user: {
    id: number;
    email: string;
    is_admin: boolean;
  };
  message: string;
}

export interface VenueMatch {
  name: string;
  city: string;
  state: string;
  existing_id?: number;
  will_create: boolean;
}

export interface ArtistMatch {
  name: string;
  position: number;
  set_type: string;
  existing_id?: number;
  will_create: boolean;
}

export interface ImportPreview {
  show: {
    title: string;
    event_date: string;
    city?: string;
    state?: string;
  };
  venues: VenueMatch[];
  artists: ArtistMatch[];
  warnings: string[];
  can_import: boolean;
}

export interface BulkImportPreviewResponse {
  previews: ImportPreview[];
  summary: {
    total_shows: number;
    new_artists: number;
    new_venues: number;
    existing_artists: number;
    existing_venues: number;
    warning_count: number;
    can_import_all: boolean;
  };
}

export interface BulkImportResult {
  success: boolean;
  show?: Show;
  error?: string;
}

export interface BulkImportConfirmResponse {
  results: BulkImportResult[];
  success_count: number;
  error_count: number;
}

export interface BulkExportResponse {
  exports: string[];
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public body?: string
  ) {
    super(`API Error: ${status} ${statusText}`);
    this.name = 'ApiError';
  }
}

export class ApiClient {
  private baseUrl: string;
  private environmentKey: string;

  constructor(baseUrl: string, environmentKey: string) {
    this.baseUrl = baseUrl;
    this.environmentKey = environmentKey;
  }

  private getToken(): string | null {
    const stored = getStoredToken(this.environmentKey);
    return stored?.token ?? null;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    requireAuth = true
  ): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (requireAuth) {
      const token = this.getToken();
      if (!token) {
        throw new ApiError(401, 'Unauthorized', 'No token available');
      }
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (!response.ok) {
      const bodyText = await response.text().catch(() => '');
      throw new ApiError(response.status, response.statusText, bodyText);
    }

    return response.json();
  }

  async login(email: string, password: string): Promise<LoginResponse> {
    // Note: Email/password login in CLI is limited because the backend uses HTTP-only cookies.
    // The recommended approach is to use the token paste flow from the web UI.
    // This login method works for environments where CORS allows cookie-based auth,
    // but the token won't be stored for subsequent requests.
    const response = await this.request<LoginResponse>(
      'POST',
      '/auth/login',
      { email, password },
      false
    );

    return response;
  }

  async getProfile(): Promise<ProfileResponse> {
    return this.request<ProfileResponse>('GET', '/auth/profile');
  }

  async getAdminShows(params?: {
    limit?: number;
    offset?: number;
    status?: string;
    from_date?: string;
    to_date?: string;
    city?: string;
  }): Promise<ShowsResponse> {
    const searchParams = new URLSearchParams();
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.offset) searchParams.set('offset', params.offset.toString());
    if (params?.status) searchParams.set('status', params.status);
    if (params?.from_date) searchParams.set('from_date', params.from_date);
    if (params?.to_date) searchParams.set('to_date', params.to_date);
    if (params?.city) searchParams.set('city', params.city);

    const query = searchParams.toString();
    const path = `/admin/shows${query ? `?${query}` : ''}`;
    return this.request<ShowsResponse>('GET', path);
  }

  async bulkExportShows(showIds: number[]): Promise<BulkExportResponse> {
    return this.request<BulkExportResponse>('POST', '/admin/shows/export/bulk', {
      show_ids: showIds,
    });
  }

  async bulkImportPreview(shows: string[]): Promise<BulkImportPreviewResponse> {
    return this.request<BulkImportPreviewResponse>(
      'POST',
      '/admin/shows/import/bulk/preview',
      { shows }
    );
  }

  async bulkImportConfirm(shows: string[]): Promise<BulkImportConfirmResponse> {
    return this.request<BulkImportConfirmResponse>(
      'POST',
      '/admin/shows/import/bulk/confirm',
      { shows }
    );
  }

  isAuthenticated(): boolean {
    return this.getToken() !== null;
  }
}
