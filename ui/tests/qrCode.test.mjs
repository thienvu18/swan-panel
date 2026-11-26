import test from 'node:test';
import assert from 'node:assert/strict';
import { encodeQR } from '../src/lib/qrcode.ts';

function assertFinderPattern(matrix, top, left) {
  for (let r = 0; r < 7; r++) {
    for (let c = 0; c < 7; c++) {
      const isOuter = r === 0 || r === 6 || c === 0 || c === 6;
      const isInner = r >= 2 && r <= 4 && c >= 2 && c <= 4;
      assert.equal(
        matrix[top + r][left + c],
        isOuter || isInner,
        `Finder pattern mismatch at (${top + r}, ${left + c})`,
      );
    }
  }
}

test('encodeQR produces valid ISO-18004 square matrices for otpauth TOTP provisioning URIs', () => {
  const uri = 'otpauth://totp/SwanPanel:alice.chen%40enterprise.corp?secret=JBSWY3DPEHPK3PXP&issuer=SwanPanel&algorithm=SHA1&digits=6&period=30';
  const matrix = encodeQR(uri);

  assert.ok(matrix.length >= 37, `Expected Version 5+ matrix for ~117 byte URI, got ${matrix.length}`);
  assert.equal((matrix.length - 17) % 4, 0, 'Matrix size must follow 17 + 4*version');
  for (const row of matrix) {
    assert.equal(row.length, matrix.length, 'QR matrix must be square');
  }

  // Three 7x7 finder patterns
  assertFinderPattern(matrix, 0, 0);
  assertFinderPattern(matrix, 0, matrix.length - 7);
  assertFinderPattern(matrix, matrix.length - 7, 0);

  // Dark module at (size - 8, 8)
  assert.equal(matrix[matrix.length - 8][8], true);

  // Timing patterns along row 6 and col 6
  for (let i = 8; i < matrix.length - 8; i++) {
    const expected = i % 2 === 0;
    assert.equal(matrix[6][i], expected, `Horizontal timing mismatch at col ${i}`);
    assert.equal(matrix[i][6], expected, `Vertical timing mismatch at row ${i}`);
  }
});

test('encodeQR scales across Version 1 and Version 10+ lengths and rejects oversized input', () => {
  const shortQR = encodeQR('hello');
  assert.equal(shortQR.length, 21); // Version 1

  const longUri = 'otpauth://totp/' + 'a'.repeat(220);
  const longQR = encodeQR(longUri);
  assert.ok(longQR.length >= 61, `Expected Version 11+ matrix for 235 byte URI, got ${longQR.length}`);

  assert.throws(() => encodeQR('x'.repeat(300)), /exceeds maximum QR capacity/);
});
