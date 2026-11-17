import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = name => readFileSync(join(root, 'pages', name), 'utf8');

test('OIDC approval submits selected real user group IDs', () => {
  const ui = source('UsersPage.tsx');
  assert.match(ui, /user_group_ids: groupIDs/);
  assert.match(ui, /pendingMemberships\[u.id\]/);
  assert.match(ui, /userGroups\.map\(group/);
  assert.doesNotMatch(ui, /defaultChecked.*Backend/);
});

test('access rule editor uses existing backend IDs for both endpoints', () => {
  const ui = source('AccessRulesPage.tsx');
  assert.match(ui, /subject_id: subjectID/);
  assert.match(ui, /value=\{subject\.id\}/);
  assert.match(ui, /value=\{group\.id\}/);
  assert.match(ui, /setSubjectID\(''\)/);
  assert.match(ui, /setTargetValue\(''\)/);
  assert.doesNotMatch(ui, /Group Name or User Email/);
});
