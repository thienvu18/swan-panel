// Validate the development environment contract without building containers.
// Real strongSwan, XFRM and LAN reachability still require runtime tests.
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

const read = (file) => readFileSync(new URL(`../${file}`, import.meta.url), 'utf8');
const config = JSON.parse(read('.devcontainer/devcontainer.json'));
const compose = read('.devcontainer/compose.yml');
const dockerfile = read('.devcontainer/Dockerfile');
const air = read('.devcontainer/air.toml');
const supervisor = read('.devcontainer/supervisord.conf');
const vite = read('ui/vite.config.ts');
const init = read('.devcontainer/start.sh');

assert.equal(config.service, 'workspace');
assert.equal(config.workspaceFolder, '/workspace/swan-panel');
assert.equal(config.dockerComposeFile, 'compose.yml');
assert.equal(config.overrideCommand, false);

for (const port of ['3000:3000/tcp', '8080:8080/tcp', '500:500/udp', '4500:4500/udp']) {
  assert.ok(compose.includes(port), `missing LAN port publishing ${port}`);
}
assert.match(compose, /SWAN_DEV_DNS_PORT.*1053.*53\/udp/);
assert.ok(compose.includes('NET_ADMIN'), 'strongSwan/firewall require Linux NET_ADMIN');
assert.ok(compose.includes('NET_RAW'), 'IKEv2 requires raw network operations');
assert.doesNotMatch(compose, /^\s*privileged:\s*true\s*$/m, 'avoid privileged container mode');
assert.ok(!compose.includes('/var/run/docker.sock'), 'avoid host Docker socket exposure');
assert.ok(compose.includes('..:/workspace/swan-panel'), 'source edits must be bind-mounted');
assert.ok(compose.includes('swanpanel_dev_data:/var/lib/swan-panel'), 'PKI/SQLite must be persistent');

for (const tool of ['strongswan', 'supervisor', 'nftables', 'build-base']) {
  assert.ok(dockerfile.includes(tool), `missing Linux runtime/build tool: ${tool}`);
}
assert.ok(dockerfile.includes('github.com/air-verse/air@'), 'Air must be installed in the dev image only');
assert.ok(!read('go.mod').includes('github.com/air-verse/air'), 'Air must not become a production dependency');

for (const service of ['strongswan', 'backend', 'frontend']) {
  assert.ok(supervisor.includes(`[program:${service}]`), `missing supervised ${service} process`);
}
assert.ok(air.includes('CGO_ENABLED=1 go build'), 'Air must compile native SQLite with CGO');
assert.ok(air.includes('entrypoint = ["/tmp/swanpanel-air/server"]'), 'use the supported Air entrypoint API');
assert.ok(air.includes('poll = true'), 'Air must poll for Docker Desktop bind-mount changes');
assert.ok(vite.includes("host: '0.0.0.0'"), 'Vite must bind a LAN-accessible interface');
assert.ok(vite.includes('port: 3000'), 'Vite must preserve the published HMR port');
assert.ok(vite.includes("'/api':"), 'Vite must proxy same-origin API calls');
assert.ok(vite.includes('changeOrigin: false'), 'Vite must preserve LAN browser Host header');
assert.ok(init.includes('ui/dist/index.html'), 'Go embed must have an index.html for dev compile');
assert.ok(init.includes('SWANPANEL_DEV_UI_PLACEHOLDER'), 'dev placeholder must be recognizable and rejected by production');
assert.ok(init.includes('openssl rand -hex 32'), 'LAN API requires random dev JWT secret');

assert.ok(existsSync(new URL('../.devcontainer/README.md', import.meta.url)));
console.log('PASS: Dev Container source/port/HMR/security configuration checks');
