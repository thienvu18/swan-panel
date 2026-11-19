import test from 'node:test';
import assert from 'node:assert/strict';
import { isValidDNSOverrideTarget } from '../src/lib/dnsRecordTargets.ts';

test('A record targets must be valid IPv4 addresses', () => {
  for (const address of ['192.168.1.100', '10.200.1.1', '0.0.0.0']) {
    assert.equal(isValidDNSOverrideTarget('A', address), true, address);
  }
  for (const address of ['gateway.enterprise.corp', '256.1.2.3', '192.168.1', '01.1.2.3', '1.2.3.4.5']) {
    assert.equal(isValidDNSOverrideTarget('A', address), false, address);
  }
});

test('CNAME targets must be hostnames, not IP literals', () => {
  for (const hostname of ['gateway.enterprise.corp', 'db-primary.acme-corp.internal', 'gateway.enterprise.corp.']) {
    assert.equal(isValidDNSOverrideTarget('CNAME', hostname), true, hostname);
  }
  for (const hostname of ['192.168.1.100', 'gateway..corp', '-gateway.corp', 'gateway.corp/extra', 'gateway']) {
    assert.equal(isValidDNSOverrideTarget('CNAME', hostname), false, hostname);
  }
});
