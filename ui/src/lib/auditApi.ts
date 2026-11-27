import { downloadFilename } from './transportSecurity.ts';

export interface AuditExportFilters {
  severity?: string;
  action?: string;
  search?: string;
}

/**
 * Exports filtered audit trail records using the authenticated Authorization header
 * and triggers a safe browser download with sanitized filename and no-store cache.
 */
export async function exportAuditLogs(
  format: 'csv' | 'jsonl',
  filters: AuditExportFilters = {},
  fetcher: typeof fetch = fetch,
): Promise<string> {
  const token = localStorage.getItem('token');
  if (!token) {
    throw new Error('Authentication required');
  }
  const params = new URLSearchParams({ format });
  if (filters.severity && filters.severity !== 'all') {
    params.set('severity', filters.severity);
  }
  if (filters.action && filters.action !== 'all') {
    params.set('action', filters.action);
  }
  if (filters.search) {
    params.set('search', filters.search);
  }

  const response = await fetcher(`/api/audit-logs/export?${params.toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || 'Failed to export audit logs');
  }
  const blob = await response.blob();
  const filename = downloadFilename(
    response.headers.get('Content-Disposition'),
    `swanpanel-audit.${format}`,
  );
  const url = URL.createObjectURL(blob);
  try {
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return filename;
}
