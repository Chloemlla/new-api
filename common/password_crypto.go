package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	passwordEncryptionKeyBits  = 2048
	passwordEncryptionNonceTTL = 5 * time.Minute

	// passwordEncryptionV2MaxBytes bounds the AES-GCM ciphertext of a v2
	// payload. The GCM plaintext is the JSON envelope, so the bound covers the
	// worst-case escaping of the password (six bytes per character), the
	// 64-character nonce, the JSON syntax and the 16-byte GCM tag.
	passwordEncryptionV2MaxBytes = MaxAccountPasswordLength*6 + 128
)

var ErrPasswordEncryptionInvalid = errors.New("password encryption payload is invalid")

// passwordEncryptionPayload is the JSON envelope encrypted with the public key.
// The nonce binds each ciphertext to a single server-issued login attempt so a
// captured ciphertext cannot be replayed.
type passwordEncryptionPayload struct {
	Nonce    string `json:"nonce"`
	Password string `json:"password"`
}

var passwordEncryptionState struct {
	sync.RWMutex
	privateKey *rsa.PrivateKey
	publicKey  string
	keyID      string
}

// GeneratePasswordEncryptionPrivateKey creates the server key used to decrypt
// browser login passwords. The caller is responsible for persisting the PEM.
func GeneratePasswordEncryptionPrivateKey() (string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, passwordEncryptionKeyBits)
	if err != nil {
		return "", fmt.Errorf("generate password encryption key: %w", err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", fmt.Errorf("marshal password encryption key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})), nil
}

// LoadPasswordEncryptionPrivateKey validates a persisted key before replacing
// the active in-memory key used by request handlers.
func LoadPasswordEncryptionPrivateKey(privateKeyPEM string) error {
	block, rest := pem.Decode([]byte(privateKeyPEM))
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return errors.New("password encryption key is not valid PKCS#8 PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse password encryption key: %w", err)
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return errors.New("password encryption key is not RSA")
	}
	if privateKey.N == nil || privateKey.N.BitLen() < passwordEncryptionKeyBits {
		return fmt.Errorf("password encryption key must be at least %d bits", passwordEncryptionKeyBits)
	}
	if err := privateKey.Validate(); err != nil {
		return fmt.Errorf("validate password encryption key: %w", err)
	}
	privateKey.Precompute()

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal password encryption public key: %w", err)
	}
	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	}))
	keyDigest := sha256.Sum256(publicKeyDER)
	keyID := hex.EncodeToString(keyDigest[:16])

	passwordEncryptionState.Lock()
	defer passwordEncryptionState.Unlock()
	passwordEncryptionState.privateKey = privateKey
	passwordEncryptionState.publicKey = publicKeyPEM
	passwordEncryptionState.keyID = keyID
	return nil
}

// PasswordEncryptionPublicKey returns the active key identifier and SPKI PEM
// public key exposed to browser clients.
func PasswordEncryptionPublicKey() (keyID string, publicKeyPEM string) {
	passwordEncryptionState.RLock()
	defer passwordEncryptionState.RUnlock()
	return passwordEncryptionState.keyID, passwordEncryptionState.publicKey
}

// DecryptPassword decrypts a base64 password ciphertext submitted by a browser.
// Legacy ciphertexts are raw RSA-OAEP/SHA-256, while v2 ciphertexts wrap a fresh
// AES-256 key with RSA-OAEP and carry the password with GCM so long Unicode
// passwords work with the existing 2048-bit server key. Both formats carry the
// same plaintext: a passwordEncryptionPayload whose nonce is issued by this
// server and has not already been consumed; anything else is treated as invalid.
// All malformed inputs share one error so callers do not expose cryptographic
// details to unauthenticated clients.
func DecryptPassword(ciphertextBase64 string, keyID string) (string, error) {
	passwordEncryptionState.RLock()
	privateKey := passwordEncryptionState.privateKey
	activeKeyID := passwordEncryptionState.keyID
	passwordEncryptionState.RUnlock()
	if privateKey == nil || keyID == "" || keyID != activeKeyID {
		return "", ErrPasswordEncryptionInvalid
	}
	if strings.HasPrefix(ciphertextBase64, "v2.") {
		if len(ciphertextBase64) > 4096 {
			return "", ErrPasswordEncryptionInvalid
		}
		parts := strings.Split(ciphertextBase64, ".")
		if len(parts) != 4 {
			return "", ErrPasswordEncryptionInvalid
		}
		wrappedKey, err := base64.StdEncoding.Strict().DecodeString(parts[1])
		if err != nil || len(wrappedKey) != privateKey.Size() {
			return "", ErrPasswordEncryptionInvalid
		}
		nonce, err := base64.StdEncoding.Strict().DecodeString(parts[2])
		if err != nil || len(nonce) != 12 {
			return "", ErrPasswordEncryptionInvalid
		}
		ciphertext, err := base64.StdEncoding.Strict().DecodeString(parts[3])
		if err != nil || len(ciphertext) <= 16 || len(ciphertext) > passwordEncryptionV2MaxBytes {
			return "", ErrPasswordEncryptionInvalid
		}
		key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, wrappedKey, []byte("password-v2"))
		if err != nil || len(key) != 32 {
			return "", ErrPasswordEncryptionInvalid
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", ErrPasswordEncryptionInvalid
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", ErrPasswordEncryptionInvalid
		}
		plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte("password-v2:"+keyID))
		if err != nil {
			return "", ErrPasswordEncryptionInvalid
		}
		return decodePasswordEncryptionPayload(plaintext, keyID)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil || len(ciphertext) != privateKey.Size() {
		return "", ErrPasswordEncryptionInvalid
	}
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, nil)
	if err != nil || len(plaintext) == 0 {
		return "", ErrPasswordEncryptionInvalid
	}
	return decodePasswordEncryptionPayload(plaintext, keyID)
}

// decodePasswordEncryptionPayload validates a decrypted envelope and consumes
// its one-time nonce. Unknown, expired, replayed and malformed payloads are all
// reported as one error so callers do not learn why a login attempt failed.
func decodePasswordEncryptionPayload(plaintext []byte, keyID string) (string, error) {
	var payload passwordEncryptionPayload
	if err := Unmarshal(plaintext, &payload); err != nil {
		return "", ErrPasswordEncryptionInvalid
	}
	if payload.Password == "" || payload.Nonce == "" {
		return "", ErrPasswordEncryptionInvalid
	}
	if !consumePasswordEncryptionNonce(payload.Nonce, keyID) {
		return "", ErrPasswordEncryptionInvalid
	}
	return payload.Password, nil
}

// ---------------------------------------------------------------------------
// One-time login nonces
// ---------------------------------------------------------------------------

type passwordEncryptionNonceRecord struct {
	keyID     string
	expiresAt time.Time
}

var passwordEncryptionNonces struct {
	sync.Mutex
	byValue map[string]passwordEncryptionNonceRecord
}

// IssuePasswordEncryptionNonce issues a single-use nonce bound to the active
// encryption key. Login ciphertexts must carry it, and it is consumed exactly
// once so a captured ciphertext cannot be replayed. Expired nonces are purged
// opportunistically on each issuance to bound the store.
func IssuePasswordEncryptionNonce() (nonce string, keyID string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	keyID, _ = PasswordEncryptionPublicKey()
	nonce = hex.EncodeToString(raw)

	passwordEncryptionNonces.Lock()
	defer passwordEncryptionNonces.Unlock()
	if passwordEncryptionNonces.byValue == nil {
		passwordEncryptionNonces.byValue = make(map[string]passwordEncryptionNonceRecord)
	}
	now := time.Now()
	for value, record := range passwordEncryptionNonces.byValue {
		if now.After(record.expiresAt) {
			delete(passwordEncryptionNonces.byValue, value)
		}
	}
	passwordEncryptionNonces.byValue[nonce] = passwordEncryptionNonceRecord{
		keyID:     keyID,
		expiresAt: now.Add(passwordEncryptionNonceTTL),
	}
	return nonce, keyID, nil
}

// consumePasswordEncryptionNonce atomically validates and consumes a one-time
// nonce. Unknown, expired, and replayed nonces all return false so callers do
// not learn why a login attempt was rejected.
func consumePasswordEncryptionNonce(nonce string, keyID string) bool {
	passwordEncryptionNonces.Lock()
	defer passwordEncryptionNonces.Unlock()
	record, ok := passwordEncryptionNonces.byValue[nonce]
	if !ok {
		return false
	}
	delete(passwordEncryptionNonces.byValue, nonce)
	return record.keyID == keyID && !time.Now().After(record.expiresAt)
}
