package common

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encryptPasswordForTest RSA-OAEP/SHA-256 encrypts a payload with the active
// public key and returns base64 ciphertext, mirroring the browser client.
func encryptPasswordForTest(t *testing.T, payload []byte) string {
	t.Helper()
	_, publicKeyPEM := PasswordEncryptionPublicKey()
	block, _ := pem.Decode([]byte(publicKeyPEM))
	require.NotNil(t, block)
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	publicKey, ok := parsed.(*rsa.PublicKey)
	require.True(t, ok)
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, payload, nil)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func setupPasswordEncryptionForTest(t *testing.T) (keyID string) {
	t.Helper()
	privateKeyPEM, err := GeneratePasswordEncryptionPrivateKey()
	require.NoError(t, err)
	require.NoError(t, LoadPasswordEncryptionPrivateKey(privateKeyPEM))
	keyID, publicKey := PasswordEncryptionPublicKey()
	require.NotEmpty(t, keyID)
	require.NotEmpty(t, publicKey)
	return keyID
}

func encryptPayloadForTest(t *testing.T, keyID string, nonce string, password string) string {
	t.Helper()
	payload, err := Marshal(passwordEncryptionPayload{Nonce: nonce, Password: password})
	require.NoError(t, err)
	return encryptPasswordForTest(t, payload)
}

func TestDecryptPasswordRoundTrip(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, issuedKeyID, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)
	require.Equal(t, keyID, issuedKeyID)
	require.Len(t, nonce, 64)

	ciphertext := encryptPayloadForTest(t, keyID, nonce, "s3cret!")
	password, err := DecryptPassword(ciphertext, keyID)
	require.NoError(t, err)
	assert.Equal(t, "s3cret!", password)
}

func TestDecryptPasswordRejectsWrongKeyID(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, _, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)
	ciphertext := encryptPayloadForTest(t, keyID, nonce, "s3cret!")

	_, err = DecryptPassword(ciphertext, "another-key-id")
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestDecryptPasswordRejectsTamperedCiphertext(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, _, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)
	ciphertext := encryptPayloadForTest(t, keyID, nonce, "s3cret!")

	tampered := ciphertext[:len(ciphertext)-2] + "ab"
	_, err = DecryptPassword(tampered, keyID)
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestDecryptPasswordRejectsInvalidBase64(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	_, err := DecryptPassword("!!!not-base64!!!", keyID)
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestDecryptPasswordConsumesNonce(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, _, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)
	ciphertext := encryptPayloadForTest(t, keyID, nonce, "s3cret!")

	_, err = DecryptPassword(ciphertext, keyID)
	require.NoError(t, err)

	// Replaying the same ciphertext must fail because its nonce was consumed.
	_, err = DecryptPassword(ciphertext, keyID)
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestDecryptPasswordRejectsExpiredNonce(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)

	expiredNonce := "expired-nonce-value"
	passwordEncryptionNonces.Lock()
	passwordEncryptionNonces.byValue[expiredNonce] = passwordEncryptionNonceRecord{
		keyID:     keyID,
		expiresAt: time.Now().Add(-time.Minute),
	}
	passwordEncryptionNonces.Unlock()

	ciphertext := encryptPayloadForTest(t, keyID, expiredNonce, "s3cret!")
	_, err := DecryptPassword(ciphertext, keyID)
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestDecryptPasswordRejectsEmptyPassword(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, _, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)
	ciphertext := encryptPayloadForTest(t, keyID, nonce, "")

	_, err = DecryptPassword(ciphertext, keyID)
	assert.ErrorIs(t, err, ErrPasswordEncryptionInvalid)
}

func TestIssuePasswordEncryptionNonceIsSingleUse(t *testing.T) {
	keyID := setupPasswordEncryptionForTest(t)
	nonce, _, err := IssuePasswordEncryptionNonce()
	require.NoError(t, err)

	assert.False(t, consumePasswordEncryptionNonce("unknown-nonce", keyID))
	assert.True(t, consumePasswordEncryptionNonce(nonce, keyID))
	assert.False(t, consumePasswordEncryptionNonce(nonce, keyID))
	assert.False(t, consumePasswordEncryptionNonce(nonce, "other-key"))
}

func TestLoadPasswordEncryptionPrivateKeyRejectsInvalidPEM(t *testing.T) {
	assert.Error(t, LoadPasswordEncryptionPrivateKey("not a pem"))
	assert.Error(t, LoadPasswordEncryptionPrivateKey("-----BEGIN PRIVATE KEY-----\ngarbage\n-----END PRIVATE KEY-----"))
	assert.Error(t, LoadPasswordEncryptionPrivateKey(""))
}

func TestLoadPasswordEncryptionPrivateKeyRejectsNonRSA(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	assert.Error(t, LoadPasswordEncryptionPrivateKey(ecPEM))
}

func TestLoadPasswordEncryptionPrivateKeyRejectsShortKey(t *testing.T) {
	shortKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(shortKey)
	require.NoError(t, err)
	shortPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	assert.Error(t, LoadPasswordEncryptionPrivateKey(shortPEM))
}

func TestPasswordEncryptionKeyIDIsStableAcrossReload(t *testing.T) {
	privateKeyPEM, err := GeneratePasswordEncryptionPrivateKey()
	require.NoError(t, err)
	require.NoError(t, LoadPasswordEncryptionPrivateKey(privateKeyPEM))
	firstID, _ := PasswordEncryptionPublicKey()
	require.NoError(t, LoadPasswordEncryptionPrivateKey(privateKeyPEM))
	secondID, _ := PasswordEncryptionPublicKey()

	assert.Equal(t, firstID, secondID)
}
