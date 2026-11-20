import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

const dir = mkdtempSync(join(tmpdir(), 'swanpanel-release-gate-'));
try {
  writeFileSync(join(dir, 'git'), '#!/bin/sh\nexit "${GIT_RESULT:-0}"\n', { mode: 0o755 });
  writeFileSync(join(dir, 'gh'), '#!/bin/sh\ncase "$*" in */artifacts*) printf "%s" "$ARTIFACTS";; *codeql.yml*) printf "%s" "$CODEQL_RUNS";; *) printf "%s" "$CI_RUNS";; esac\n', { mode: 0o755 });
  const run = (id, status = 'completed', conclusion = 'success', event = 'push') =>
    ({ id, status, conclusion, event, created_at: `2026-01-${String(id).padStart(2, '0')}T00:00:00Z` });
  const invoke = (ci, codeql, gitResult = 0, artifacts = [{ id: 10, name: 'swan-panel-ui-dist', expired: false }]) => {
    const output = join(dir, 'output');
    writeFileSync(output, '');
    const result = spawnSync('sh', [resolve('scripts/verify-release-ci.sh')], {
      encoding: 'utf8', env: { ...process.env, PATH: `${dir}:${process.env.PATH}`,
        COMMIT: 'test-sha', REPOSITORY: 'owner/repo', GITHUB_OUTPUT: output,
        GIT_RESULT: String(gitResult), CI_RUNS: JSON.stringify({ workflow_runs: ci }),
        CODEQL_RUNS: JSON.stringify({ workflow_runs: codeql }), ARTIFACTS: JSON.stringify({ artifacts }) },
    });
    return { ...result, output: readFileSync(output, 'utf8') };
  };
  const success = invoke([run(1)], [run(2)]);
  assert.equal(success.status, 0, success.stderr);
  assert.equal(success.output, 'run_id=1\nartifact_id=10\n');
  for (const [name, ci, codeql, gitResult] of [
    ['commit outside main', [run(1)], [run(2)], 1],
    ['no CI', [], [run(2)], 0],
    ['PR check only', [run(1, 'completed', 'success', 'pull_request')], [run(2)], 0],
    ['CI pending', [run(1, 'in_progress', null)], [run(2)], 0],
    ['latest CI failed', [run(1), run(3, 'completed', 'failure')], [run(2)], 0],
    ['CodeQL failed', [run(1)], [run(2, 'completed', 'failure')], 0],
  ]) {
    assert.notEqual(invoke(ci, codeql, gitResult).status, 0, name);
  }
  assert.notEqual(invoke([run(1)], [run(2)], 0, []).status, 0, 'missing artifact');
  assert.notEqual(invoke([run(1)], [run(2)], 0, [{ id: 10, name: 'swan-panel-ui-dist', expired: true }]).status, 0, 'expired artifact');
  assert.equal(invoke([run(1)], [run(2)], 0, [{ id: 10, name: 'swan-panel-ui-dist', expired: false }, { id: 11, name: 'swan-panel-ui-dist', expired: false }]).output, 'run_id=1\nartifact_id=11\n');
  assert.notEqual(invoke([run(1)], [run(2)], 0, [{ id: 10, name: 'swan-panel-ui-dist', expired: false }, { id: 11, name: 'swan-panel-ui-dist', expired: true }]).status, 0, 'latest artifact expired');
  assert.equal(invoke([run(1, 'completed', 'success', 'workflow_dispatch')], [run(2)]).status, 0);
  console.log('PASS: release requires successful main CI and CodeQL for the tagged commit');
} finally {
  rmSync(dir, { recursive: true, force: true });
}
