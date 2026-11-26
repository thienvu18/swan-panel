package mfa_test

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/swan-panel/swan-panel/internal/mfa"
)

func TestTOTPGenerationRFC6238VectorsAndReplayProtection(t *testing.T) {
	// RFC 6238 Appendix B test seed: "12345678901234567890" (20 bytes)
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	ts := time.Unix(59, 0).UTC() // step 1

	code, err := mfa.GenerateCode(secret, ts)
	if err != nil {
		t.Fatalf("generate RFC 6238 code: %v", err)
	}
	if code != "287082" {
		t.Fatalf("expected RFC 6238 SHA1 code 287082 at t=59, got %s", code)
	}

	step, ok := mfa.VerifyCode(secret, code, ts, 0)
	if !ok || step != 1 {
		t.Fatalf("expected verification at step 1, got step=%d ok=%t", step, ok)
	}
	// Replaying the same step must be rejected.
	if _, ok := mfa.VerifyCode(secret, code, ts, step); ok {
		t.Fatal("expected same-step TOTP replay to be rejected")
	}
	// +/- 1 step skew should verify when step > lastUsedStep.
	if skewStep, ok := mfa.VerifyCode(secret, code, ts.Add(30*time.Second), 0); !ok || skewStep != 1 {
		t.Fatalf("expected +/-1 skew acceptance, got step=%d ok=%t", skewStep, ok)
	}
	// Outside skew window must fail.
	if _, ok := mfa.VerifyCode(secret, code, ts.Add(90*time.Second), 0); ok {
		t.Fatal("expected out-of-window TOTP code to fail")
	}

	uri := mfa.BuildOTPAuthURI("SwanPanel VPN", "admin@example.com", secret)
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("unexpected otpauth URI: %s", uri)
	}
}

func TestSecretEncryptionRoundTripAndWrongKeyRejection(t *testing.T) {
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	enc, err := mfa.EncryptSecret(secret, "primary-root-secret-0123456789abcdef")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if enc == secret || strings.Contains(enc, secret) {
		t.Fatal("encrypted payload exposed plaintext secret")
	}
	dec, err := mfa.DecryptSecret(enc, "primary-root-secret-0123456789abcdef")
	if err != nil || dec != secret {
		t.Fatalf("decrypt mismatch: got=%q err=%v", dec, err)
	}
	if _, err := mfa.DecryptSecret(enc, "wrong-secret"); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}

func TestBackupCodesGenerationAndSingleUseConsumption(t *testing.T) {
	root := "backup-root-secret"
	codes, digests, err := mfa.GenerateBackupCodes(root)
	if err != nil {
		t.Fatalf("generate backup codes: %v", err)
	}
	if len(codes) != 8 || mfa.CountBackupCodes(digests) != 8 {
		t.Fatalf("expected 8 backup codes, got len=%d count=%d", len(codes), mfa.CountBackupCodes(digests))
	}

	// Accept lowercase without hyphens and consume atomically.
	normalizedCandidate := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
	remaining, ok := mfa.ConsumeBackupCode(digests, normalizedCandidate, root)
	if !ok || mfa.CountBackupCodes(remaining) != 7 {
		t.Fatalf("expected first backup code consumption to leave 7 codes, ok=%t count=%d", ok, mfa.CountBackupCodes(remaining))
	}
	// Reusing the same code must fail.
	if _, ok := mfa.ConsumeBackupCode(remaining, codes[0], root); ok {
		t.Fatal("expected consumed backup code replay to fail")
	}
}
