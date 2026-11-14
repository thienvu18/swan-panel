package pki

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/smallstep/pkcs7"
)

// SignAppleProfile creates an *attached* CMS/PKCS#7 SignedData document.
// Apple accepts this DER representation as a signed .mobileconfig.
// Signing protects integrity/authenticity, but DOES NOT encrypt profile secrets.
// Apple displays "Verified" only when the signing chain is trusted by the device.
func (m *PKIManager) SignAppleProfile(xml []byte) ([]byte, error) {
	if len(xml) == 0 {
		return nil, errors.New("cannot sign empty Apple profile")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	signer, privateKey, err := m.loadOrCreateProfileSigner()
	if err != nil {
		return nil, err
	}
	signed, err := pkcs7.NewSignedData(xml)
	if err != nil {
		return nil, fmt.Errorf("start CMS signing: %w", err)
	}
	// pkcs7 defaults to SHA-1 unless explicitly overridden.
	signed.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := signed.AddSignerChain(signer, privateKey, []*x509.Certificate{m.caCert}, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, fmt.Errorf("add profile signer: %w", err)
	}
	der, err := signed.Finish()
	if err != nil {
		return nil, fmt.Errorf("finish CMS profile signing: %w", err)
	}

	// Reject serialization failures and altered content rather than serving unsigned data.
	parsed, err := pkcs7.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("parse signed Apple profile: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(m.caCert)
	if err := parsed.VerifyWithChain(roots); err != nil {
		return nil, fmt.Errorf("verify signed Apple profile: %w", err)
	}
	if !bytes.Equal(parsed.Content, xml) {
		return nil, errors.New("signed Apple profile payload changed")
	}
	return der, nil
}

// Only a dedicated leaf key may sign downloadable profiles. Never sign
// arbitrary client content using the CA private key itself.
func (m *PKIManager) loadOrCreateProfileSigner() (*x509.Certificate, *rsa.PrivateKey, error) {
	certPath := filepath.Join(m.dataDir, "profile-signing.crt")
	keyPath := filepath.Join(m.dataDir, "profile-signing.key")
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		block, _ := pem.Decode(certPEM)
		if block == nil {
			return nil, nil, errors.New("stored profile signer certificate is not PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, err
		}
		key, err := parseRSAPrivateKeyFromPEM(keyPEM)
		if err != nil {
			return nil, nil, err
		}
		public, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok || public.N.Cmp(key.N) != 0 || public.E != key.E {
			return nil, nil, errors.New("stored profile signer certificate/key mismatch")
		}
		if err := cert.CheckSignatureFrom(m.caCert); err != nil {
			return nil, nil, err
		}
		if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
			return nil, nil, errors.New("profile signing certificate lacks digitalSignature")
		}
		if time.Now().After(cert.NotAfter) {
			return nil, nil, errors.New("profile signer expired; manual rotation required")
		}
		return cert, key, nil
	}
	if !os.IsNotExist(certErr) || !os.IsNotExist(keyErr) {
		return nil, nil, fmt.Errorf("profile signer material missing or unreadable: cert=%v key=%v", certErr, keyErr)
	}
	if m.caCert == nil || m.caKey == nil {
		return nil, nil, errors.New("root CA unavailable")
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, nil, err
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	expiry := now.AddDate(2, 0, 0)
	if expiry.After(m.caCert.NotAfter) {
		expiry = m.caCert.NotAfter
	}
	subjectKey := sha256.Sum256(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"SwanPanel"}, CommonName: "SwanPanel Configuration Profile Signing"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              expiry,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection, x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		SubjectKeyId:          subjectKey[:20],
	}
	der, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certEncoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := atomicPrivateFile(keyPath, privatePEM); err != nil {
		return nil, nil, err
	}
	if err := atomicPrivateFile(certPath, certEncoded); err != nil {
		_ = os.Remove(keyPath)
		return nil, nil, err
	}
	return cert, key, nil
}
