export interface AndroidNativeSetup {
  client_p12: string;
  root_ca: string;
  import_password: string;
  server_address: string;
  server_identity: string;
  client_identity: string;
}

// Fetch the PKCS#12 and its one-time password in the same authenticated
// response. Never put JWTs, PKCS#12 bytes, or passwords in query strings.
export async function fetchAndroidNativeSetup(deviceId: string, signal?: AbortSignal): Promise<AndroidNativeSetup> {
  const token = localStorage.getItem('token');
  if (!token) throw new Error('Your session has expired. Sign in again.');
  const response = await fetch(`/api/devices/${encodeURIComponent(deviceId)}/config/android-native`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
    signal,
  });
  if (!response.ok) {
    const failure = await response.json().catch(() => ({}));
    throw new Error(failure.error || 'Unable to prepare Android credentials');
  }
  return response.json() as Promise<AndroidNativeSetup>;
}

export function downloadAndroidCredential(base64: string, filename: string, contentType: string): void {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  const url = URL.createObjectURL(new Blob([bytes], { type: contentType }));
  try {
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename.replace(/[^a-zA-Z0-9_.-]/g, '-');
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
}
