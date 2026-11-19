/** Shared JSON transport for the authenticated management API. */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export interface ApiRequestOptions extends RequestInit {
  authenticated?: boolean;
}

export async function apiJSON<T>(url: string, options: ApiRequestOptions = {}): Promise<T> {
  const { authenticated = true, headers, ...request } = options;
  const nextHeaders = new Headers(headers);
  if (authenticated) {
    const token = localStorage.getItem('token');
    if (!token) throw new ApiError('Authentication required', 401);
    nextHeaders.set('Authorization', `Bearer ${token}`);
  }
  if (request.body && !nextHeaders.has('Content-Type') && typeof request.body === 'string') {
    nextHeaders.set('Content-Type', 'application/json');
  }
  const response = await fetch(url, { ...request, headers: nextHeaders, cache: request.cache ?? 'no-store' });
  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const details = body && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
      ? body.error : `Request failed (HTTP ${response.status})`;
    throw new ApiError(details, response.status);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
