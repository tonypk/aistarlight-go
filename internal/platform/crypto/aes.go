package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// AESEncryptor provides AES-256-GCM encryption and decryption.
type AESEncryptor struct {
	gcm cipher.AEAD
}

// NewAESEncryptor creates an encryptor from a 32-byte hex-encoded key.
func NewAESEncryptor(hexKey string) (*AESEncryptor, error) {
	if hexKey == "" {
		return nil, errors.New("encryption key is empty")
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes (256 bits), got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &AESEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns nonce+ciphertext.
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return e.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts nonce+ciphertext produced by Encrypt.
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptString is a convenience wrapper that encrypts a string.
func (e *AESEncryptor) EncryptString(s string) ([]byte, error) {
	return e.Encrypt([]byte(s))
}

// DecryptString is a convenience wrapper that decrypts to a string.
func (e *AESEncryptor) DecryptString(ciphertext []byte) (string, error) {
	plaintext, err := e.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
