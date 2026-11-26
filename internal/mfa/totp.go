package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// TOTPPeriodSeconds is the standard RFC 6238 30-second time step.
	TOTPPeriodSeconds = 30
	// TOTPDigits is the standard 6-digit code length.
	TOTPDigits = 6
	// totpSkewSteps allows +/- 1 time step (30s) clock skew.
	totpSkewSteps = 1

	backupCodeCount    = 8
	backupCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// GenerateSecret creates a 160-bit random Base32-encoded TOTP secret.
func GenerateSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate TOTP entropy: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// BuildOTPAuthURI formats an otpauth:// URI compatible with standard authenticator apps.
func BuildOTPAuthURI(issuer, accountEmail, secret string) string {
	cleanIssuer := strings.TrimSpace(issuer)
	if cleanIssuer == "" {
		cleanIssuer = "SwanPanel"
	}
	label := url.PathEscape(cleanIssuer) + ":" + url.PathEscape(strings.TrimSpace(accountEmail))
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", cleanIssuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", "6")
	query.Set("period", "30")
	return "otpauth://totp/" + label + "?" + query.Encode()
}

func decodeBase32Secret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(secret, " ", "")))
	if normalized == "" {
		return nil, errors.New("empty TOTP secret")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return nil, fmt.Errorf("decode base32 secret: %w", err)
	}
	if len(key) < 10 {
		return nil, errors.New("TOTP secret too short")
	}
	return key, nil
}

// GenerateCode computes the 6-digit TOTP code for the given step timestamp.
func GenerateCode(secret string, at time.Time) (string, error) {
	key, err := decodeBase32Secret(secret)
	if err != nil {
		return "", err
	}
	step := at.UTC().Unix() / TOTPPeriodSeconds
	return codeForStep(key, step), nil
}

func codeForStep(key []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))

	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)

	code := truncated % 1000000
	return fmt.Sprintf("%06d", code)
}

func normalizeDigits(code string) (string, bool) {
	cleaned := strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(cleaned) != TOTPDigits {
		return "", false
	}
	for i := 0; i < len(cleaned); i++ {
		if cleaned[i] < '0' || cleaned[i] > '9' {
			return "", false
		}
	}
	return cleaned, true
}

// VerifyCode validates a 6-digit TOTP code within the skew window and enforces
// single-use step replay protection when lastUsedStep > 0.
func VerifyCode(secret, code string, at time.Time, lastUsedStep int64) (int64, bool) {
	normalized, ok := normalizeDigits(code)
	if !ok {
		return 0, false
	}
	key, err := decodeBase32Secret(secret)
	if err != nil {
		return 0, false
	}
	currentStep := at.UTC().Unix() / TOTPPeriodSeconds
	for delta := int64(-totpSkewSteps); delta <= totpSkewSteps; delta++ {
		candidateStep := currentStep + delta
		if candidateStep <= 0 || candidateStep <= lastUsedStep {
			continue
		}
		expected := codeForStep(key, candidateStep)
		if subtle.ConstantTimeCompare([]byte(normalized), []byte(expected)) == 1 {
			return candidateStep, true
		}
	}
	return 0, false
}

func deriveAESKey(rootSecret string) []byte {
	sum := sha256.Sum256([]byte("swanpanel-mfa-aes-v1:" + rootSecret))
	return sum[:]
}

// EncryptSecret encrypts a TOTP seed using AES-256-GCM before persisting to SQLite.
func EncryptSecret(plaintext, rootSecret string) (string, error) {
	block, err := aes.NewCipher(deriveAESKey(rootSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret decrypts an AES-256-GCM encrypted TOTP seed from SQLite.
func DecryptSecret(ciphertextB64, rootSecret string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveAESKey(rootSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext truncated")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// GenerateBackupCodes generates single-use recovery codes and their stored keyed digests.
func GenerateBackupCodes(rootSecret string) ([]string, string, error) {
	plaintext := make([]string, 0, backupCodeCount)
	digests := make([]string, 0, backupCodeCount)
	buf := make([]byte, 10)
	alphabetLen := byte(len(backupCodeAlphabet))

	for i := 0; i < backupCodeCount; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, "", err
		}
		var chars [10]byte
		for j := 0; j < 10; j++ {
			chars[j] = backupCodeAlphabet[buf[j]%alphabetLen]
		}
		code := string(chars[:5]) + "-" + string(chars[5:])
		plaintext = append(plaintext, code)
		digests = append(digests, hashBackupCode(code, rootSecret))
	}

	encoded, err := json.Marshal(digests)
	if err != nil {
		return nil, "", err
	}
	return plaintext, string(encoded), nil
}

func normalizeBackupCode(raw string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(raw))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	return cleaned
}

func hashBackupCode(code, rootSecret string) string {
	mac := hmac.New(sha256.New, []byte("swanpanel-backup-v1:"+rootSecret))
	mac.Write([]byte(normalizeBackupCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ConsumeBackupCode checks a recovery code in constant time and removes it from the stored JSON list.
func ConsumeBackupCode(storedJSON, candidate, rootSecret string) (string, bool) {
	if strings.TrimSpace(storedJSON) == "" || len(normalizeBackupCode(candidate)) < 8 {
		return storedJSON, false
	}
	var digests []string
	if err := json.Unmarshal([]byte(storedJSON), &digests); err != nil {
		return storedJSON, false
	}
	target := hashBackupCode(candidate, rootSecret)
	matchedIndex := -1
	for i, existing := range digests {
		if subtle.ConstantTimeCompare([]byte(existing), []byte(target)) == 1 {
			matchedIndex = i
		}
	}
	if matchedIndex < 0 {
		return storedJSON, false
	}
	remaining := append([]string{}, digests[:matchedIndex]...)
	remaining = append(remaining, digests[matchedIndex+1:]...)
	encoded, err := json.Marshal(remaining)
	if err != nil {
		return storedJSON, false
	}
	return string(encoded), true
}

// CountBackupCodes returns the number of unused backup codes remaining.
func CountBackupCodes(storedJSON string) int {
	if strings.TrimSpace(storedJSON) == "" {
		return 0
	}
	var digests []string
	if err := json.Unmarshal([]byte(storedJSON), &digests); err != nil {
		return 0
	}
	return len(digests)
}
