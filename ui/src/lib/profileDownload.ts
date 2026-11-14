import { downloadFilename } from './transportSecurity.ts';

export async function downloadDeviceProfile(deviceId: string, platform: string, hostname: string): Promise<void> {
  const token = localStorage.getItem('token');
  if (!token) {
    throw new Error('Your session has expired. Sign in again to download a profile.');
  }

  // The backend intentionally rejects JWTs in URLs; send the credential only
  // in an Authorization header and keep sensitive profiles out of HTTP caches.
  const response = await fetch(
    `/api/devices/${encodeURIComponent(deviceId)}/config/${encodeURIComponent(platform)}`,
    {
      headers: { Authorization: `Bearer ${token}` },
      cache: 'no-store',
    },
  );
  if (!response.ok) {
    if (response.status === 401) {
      throw new Error('Your session has expired. Sign in again to download a profile.');
    }
    const error = await response.json().catch(() => ({}));
    throw new Error(error.error || 'Unable to download VPN configuration');
  }

  const profile = await response.blob();
  const filename = downloadFilename(
    response.headers.get('Content-Disposition'),
    `swanpanel-${hostname}-${platform}`,
  );
  const objectURL = URL.createObjectURL(profile);
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
}
