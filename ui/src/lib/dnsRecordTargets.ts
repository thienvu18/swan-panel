export type DNSOverrideRecordType = 'A' | 'CNAME';

const dnsLabel = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;

export function isValidDNSOverrideTarget(type: DNSOverrideRecordType, input: string): boolean {
  const value = input.trim();
  if (type === 'A') {
    const octets = value.split('.');
    return octets.length === 4 && octets.every((octet) =>
      /^(0|[1-9]\d{0,2})$/.test(octet) && Number(octet) <= 255
    );
  }

  if (type === 'CNAME') {
    const hostname = value.endsWith('.') ? value.slice(0, -1) : value;
    if (/^\d+\.\d+\.\d+\.\d+$/.test(hostname)) return false;
    return hostname.length > 0 && hostname.length <= 253
      && hostname.includes('.') && hostname.split('.').every((label) => dnsLabel.test(label));
  }
  return false;
}
