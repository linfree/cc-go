package wechat

import (
	"crypto/aes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
)

// UploadResult holds the outcome of the full CDN upload flow.
type UploadResult struct {
	DownloadParam       string // becomes encrypt_query_param in CDNMedia
	AESKeyBase64        string // becomes aes_key in CDNMedia
	AESKeyHex           string // hex representation (used in getuploadurl request)
	FileSize            int    // plaintext file size
	FileSizeCiphertext int    // ciphertext size after AES-128-ECB + PKCS7
}

// CDNMedia appears in image_item, file_item, video_item payloads.
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param"`
	AESKey            string `json:"aes_key"`
	EncryptType       int    `json:"encrypt_type"`
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

func aesEncryptECB(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return ciphertext, nil
}

func computeCiphertextSize(plaintextSize int) int {
	return ((plaintextSize + 1) / 16 + 1) * 16
}

func fileMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

func generateAESKey() (rawKey []byte, hexKey string, err error) {
	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(rand.Uint32())
	}
	return key, hex.EncodeToString(key), nil
}

func generateFileKey() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rand.Uint32())
	}
	return hex.EncodeToString(b)
}

func aesKeyToBase64(hexKey string) string {
	b, _ := hex.DecodeString(hexKey)
	return base64.StdEncoding.EncodeToString(b)
}