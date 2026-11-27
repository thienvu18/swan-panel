import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');

function extractTagById(source, id) {
  const idToken = `id="${id}"`;
  const idx = source.indexOf(idToken);
  assert.notEqual(idx, -1, `Expected element with ${idToken}`);
  const start = source.lastIndexOf('<', idx);
  const end = source.indexOf('/>', idx);
  assert.notEqual(start, -1, `Could not find opening bracket for ${id}`);
  assert.notEqual(end, -1, `Could not find self-closing end for ${id}`);
  return source.slice(start, end + 2);
}

test('LoginPage handles two-factor challenges with an accessible one-time-code input supporting recovery codes', () => {
  const login = readFileSync(join(srcRoot, 'pages/LoginPage.tsx'), 'utf8');
  assert.match(login, /\/api\/auth\/mfa\/verify/);
  assert.match(login, /data\.mfa_required && data\.mfa_token/);
  assert.match(login, /method="post"/);
  const otpTag = extractTagById(login, 'mfa-code-input');
  assert.match(otpTag, /name="code"/);
  assert.match(otpTag, /type="text"/);
  assert.match(otpTag, /inputMode="text"/);
  assert.match(otpTag, /autoComplete="one-time-code"/);
  assert.match(otpTag, /autoCorrect="off"/);
  assert.match(otpTag, /spellCheck=\{false\}/);
  assert.ok(
    login.indexOf('id="mfa-login-hint"') < login.indexOf('id="mfa-code-input"'),
    'OTP hint should appear above the input so mobile keyboards do not cover it',
  );
});

test('LoginPage and UsersPage provide standard browser autofill attributes for username, email, name, and password', () => {
  const login = readFileSync(join(srcRoot, 'pages/LoginPage.tsx'), 'utf8');

  const emailTag = extractTagById(login, 'email-input');
  assert.match(emailTag, /name="email"/);
  assert.match(emailTag, /type="email"/);
  assert.match(emailTag, /inputMode="email"/);
  assert.match(emailTag, /autoComplete=\{setupRequired \? 'email' : 'username'\}/);
  assert.match(emailTag, /autoCapitalize="none"/);
  assert.match(emailTag, /autoCorrect="off"/);
  assert.match(emailTag, /spellCheck=\{false\}/);

  const loginPasswordTag = extractTagById(login, 'password-input');
  assert.match(loginPasswordTag, /name="password"/);
  assert.match(loginPasswordTag, /autoComplete=\{setupRequired \? 'new-password' : 'current-password'\}/);

  const adminNameTag = extractTagById(login, 'admin-name');
  assert.match(adminNameTag, /name="name"/);
  assert.match(adminNameTag, /type="text"/);
  assert.match(adminNameTag, /autoComplete="name"/);

  const users = readFileSync(join(srcRoot, 'pages/UsersPage.tsx'), 'utf8');
  const createNameTag = extractTagById(users, 'create-user-name');
  assert.match(createNameTag, /name="name"/);
  assert.match(createNameTag, /type="text"/);
  assert.match(createNameTag, /autoComplete="name"/);

  const createEmailTag = extractTagById(users, 'create-user-email');
  assert.match(createEmailTag, /name="email"/);
  assert.match(createEmailTag, /type="email"/);
  assert.match(createEmailTag, /inputMode="email"/);
  assert.match(createEmailTag, /autoComplete="email"/);
  assert.match(createEmailTag, /autoCapitalize="none"/);
  assert.match(createEmailTag, /autoCorrect="off"/);

  const createPasswordTag = extractTagById(users, 'create-user-password');
  assert.match(createPasswordTag, /name="password"/);
  assert.match(createPasswordTag, /type="password"/);
  assert.match(createPasswordTag, /autoComplete="new-password"/);
});

test('AccountSecurityModal exposes balanced QR-above-secret TOTP setup and complete browser autofill attributes', () => {
  const modal = readFileSync(join(srcRoot, 'components/AccountSecurityModal.tsx'), 'utf8');
  const sidebar = readFileSync(join(srcRoot, 'components/Sidebar.tsx'), 'utf8');
  const app = readFileSync(join(srcRoot, 'App.tsx'), 'utf8');

  assert.match(modal, /\/api\/auth\/mfa\/setup/);
  assert.match(modal, /\/api\/auth\/mfa\/enable/);
  assert.match(modal, /\/api\/auth\/mfa\/disable/);
  assert.match(modal, /\/api\/auth\/mfa\/backup-codes/);

  // QR code rendered above secret key and verification code input
  const qrPos = modal.indexOf('<QRCodeSVG');
  const secretPos = modal.indexOf('{setupPayload.secret}');
  const codePos = modal.indexOf('id="mfa-enroll-code"');
  assert.notEqual(qrPos, -1, 'AccountSecurityModal must render QRCodeSVG');
  assert.ok(qrPos < secretPos, 'Secret key must be placed below the QR code');
  assert.ok(secretPos < codePos, 'Verification code input must appear below the secret key');

  const enrollCodeTag = extractTagById(modal, 'mfa-enroll-code');
  assert.match(enrollCodeTag, /name="code"/);
  assert.match(enrollCodeTag, /inputMode="numeric"/);
  assert.match(enrollCodeTag, /autoComplete="one-time-code"/);
  assert.match(enrollCodeTag, /text-center/);

  const manageCodeTag = extractTagById(modal, 'mfa-manage-code');
  assert.match(manageCodeTag, /name="code"/);
  assert.match(manageCodeTag, /autoComplete="one-time-code"/);

  for (const pwId of ['mfa-setup-password', 'mfa-manage-password', 'sec-current-pw']) {
    const tag = extractTagById(modal, pwId);
    assert.match(tag, /name="current_password"/);
    assert.match(tag, /type="password"/);
    assert.match(tag, /autoComplete="current-password"/);
  }

  const newPwTag = extractTagById(modal, 'sec-new-pw');
  assert.match(newPwTag, /name="new_password"/);
  assert.match(newPwTag, /type="password"/);
  assert.match(newPwTag, /autoComplete="new-password"/);

  const confirmPwTag = extractTagById(modal, 'sec-confirm-pw');
  assert.match(confirmPwTag, /name="confirm_password"/);
  assert.match(confirmPwTag, /type="password"/);
  assert.match(confirmPwTag, /autoComplete="new-password"/);

  assert.match(modal, /autoComplete="username"/, 'Password/2FA forms must provide hidden username context for password managers');
  assert.match(sidebar, /onOpenSecurity/);
  assert.match(app, /AccountSecurityModal/);
});

test('Admin role, OIDC SSO, and 2FA actions use distinct icons', () => {
  const users = readFileSync(join(srcRoot, 'pages/UsersPage.tsx'), 'utf8');
  const sidebar = readFileSync(join(srcRoot, 'components/Sidebar.tsx'), 'utf8');
  const modal = readFileSync(join(srcRoot, 'components/AccountSecurityModal.tsx'), 'utf8');

  // UsersPage should distinguish Admin privilege (Crown) from 2FA reset (RotateCcwKey)
  assert.match(users, /<Crown\b/);
  assert.match(users, /<RotateCcwKey\b/);
  assert.doesNotMatch(users, /<ShieldOff\b/);
  assert.doesNotMatch(users, /<Shield\b/);

  // Sidebar account security button must not duplicate OIDC KeyRound icon
  assert.match(sidebar, /icon:\s*KeyRound/);
  assert.match(sidebar, /<UserLock\b/);
  assert.doesNotMatch(sidebar.slice(sidebar.indexOf('onOpenSecurity')), /<KeyRound\b/);

  // AccountSecurityModal header uses Fingerprint rather than Access Rules ShieldCheck
  assert.match(modal, /<Fingerprint\b/);
  assert.match(modal, /<QrCode\b/);
});
