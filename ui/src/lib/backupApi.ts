import { ApiError } from './api.ts';
import { downloadFilename } from './transportSecurity.ts';

export interface RestoreBackupResponse {
  restore_id?: string;
  runtime_status: 'applied' | 'pending' | 'restarting' | string;
  maintenance?: boolean;
  retry_after_seconds?: number;
  message: string;
  manifest?: {
    format_version: number;
    created_at: string;
    db_sha256: string;
    pki_files?: string[];
  };
}

export class RestoreOutcomeError extends Error {
  outcome: 'rolled_back' | 'failed_closed' | 'timeout' | 'unverified';

  constructor(outcome: 'rolled_back' | 'failed_closed' | 'timeout' | 'unverified', message: string) {
    super(message);
    this.name = 'RestoreOutcomeError';
    this.outcome = outcome;
  }
}

function getAuthToken(): string {
  const token = localStorage.getItem('token');
  if (!token) {
    throw new ApiError('Authentication required', 401);
  }
  return token;
}

async function readErrorMessage(response: Response, fallback: string): Promise<string> {
  const body: unknown = await response.json().catch(() => null);
  if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string' && body.error.trim()) {
    return body.error;
  }
  return `${fallback} (HTTP ${response.status})`;
}

/** Export an encrypted archive containing panel database and VPN certificate authority state. */
export async function downloadEncryptedBackup(
  passphrase: string,
  fetchImpl: typeof fetch = fetch,
): Promise<string> {
  const token = getAuthToken();
  const response = await fetchImpl('/api/system/backup', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ passphrase }),
    cache: 'no-store',
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response, 'Backup export failed'), response.status);
  }

  const blob = await response.blob();
  const dateStamp = new Date().toISOString().slice(0, 10);
  const filename = downloadFilename(
    response.headers.get('Content-Disposition'),
    `swanpanel-backup-${dateStamp}.tar.enc`,
  );
  const objectURL = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement('a');
    anchor.href = objectURL;
    anchor.download = filename;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
  }
  return filename;
}

/** Upload and restore an encrypted SwanPanel backup archive. */
export async function uploadRestoreBackup(
  file: File,
  passphrase: string,
  confirmPhrase: string,
  fetchImpl: typeof fetch = fetch,
): Promise<RestoreBackupResponse> {
  const token = getAuthToken();
  const formData = new FormData();
  formData.append('backup_file', file);
  formData.append('passphrase', passphrase);
  formData.append('confirm', confirmPhrase);

  const response = await fetchImpl('/api/system/restore', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: formData,
    cache: 'no-store',
  });
  if (!response.ok) {
    throw new ApiError(await readErrorMessage(response, 'Backup restore failed'), response.status);
  }
  return response.json() as Promise<RestoreBackupResponse>;
}

/**
 * Bounded wait-and-reconnect probe invoked strictly after an initiated controlled restore
 * restart until /api/health returns healthy HTTP 200 and correlates the restore outcome.
 */
export async function waitForControlPlaneRestart(
  maxAttempts = 30,
  delayMs = 2000,
  fetchImpl: typeof fetch = fetch,
  restoreId?: string,
): Promise<'applied'> {
  const healthURL = restoreId
    ? `/api/health?restore_id=${encodeURIComponent(restoreId)}`
    : '/api/health';

  let sawUnverifiedHealthy = false;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    await new Promise(resolve => setTimeout(resolve, delayMs));
    try {
      const resp = await fetchImpl(healthURL, { cache: 'no-store' });
      const body = (await resp.json().catch(() => null)) as {
        status?: string;
        mode?: string;
        maintenance?: boolean;
        restore_status?: string;
        restore_error?: string;
      } | null;

      if (body?.mode === 'failed_closed' || body?.restore_status === 'failed_closed') {
        throw new RestoreOutcomeError(
          'failed_closed',
          body.restore_error || 'Backup restore and rollback both failed; manual operator inspection required',
        );
      }

      if (resp.ok && body && body.status === 'healthy' && !body.maintenance) {
        if (body.restore_status === 'rolled_back') {
          throw new RestoreOutcomeError(
            'rolled_back',
            body.restore_error || 'Backup restore failed during activation and rolled back to the prior state',
          );
        }
        // Never assume applied when restoreId correlation was requested unless explicitly confirmed.
        if (!restoreId || body.restore_status === 'applied') {
          return 'applied';
        }
        sawUnverifiedHealthy = true;
      }
    } catch (err) {
      if (err instanceof RestoreOutcomeError) {
        throw err;
      }
      // Expected while the control-plane process shuts down and restarts cleanly.
    }
  }
  if (sawUnverifiedHealthy) {
    throw new RestoreOutcomeError(
      'unverified',
      'Control plane became healthy, but restore outcome confirmation for this operation was unavailable',
    );
  }
  throw new RestoreOutcomeError('timeout', 'Control plane did not become ready within timeout after restore restart');
}
