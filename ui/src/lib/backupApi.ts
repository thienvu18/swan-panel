import { ApiError } from './api.ts';
import { downloadFilename } from './transportSecurity.ts';

export interface RestoreBackupResponse {
  runtime_status: string;
  message: string;
  manifest?: {
    format_version: number;
    created_at: string;
    db_sha256: string;
    pki_files?: string[];
  };
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
