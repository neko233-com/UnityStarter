package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

func integrityHasher(integrity string) (hash.Hash, []byte, error) {
	if integrity == "" {
		return nil, nil, nil
	}
	algorithm, encoded, ok := strings.Cut(integrity, "-")
	if !ok {
		return nil, nil, fmt.Errorf("invalid installer integrity")
	}
	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	case "sha1":
		h = sha1.New()
	case "sha384":
		h = sha512.New384()
	default:
		return nil, nil, fmt.Errorf("unsupported installer integrity algorithm")
	}
	expected, err := base64.StdEncoding.DecodeString(encoded)
	if err == nil && algorithm == "md5" && len(expected) == md5.Size*2 {
		expected, err = hex.DecodeString(string(expected))
	}
	if err != nil || len(expected) != h.Size() {
		return nil, nil, fmt.Errorf("invalid installer integrity")
	}
	return h, expected, nil
}

// Older Unity release metadata encodes the hexadecimal MD5 string in base64.
// Those releases also report rounded download sizes instead of exact bytes.
func legacyUnityMD5(integrity string) bool {
	encoded, ok := strings.CutPrefix(integrity, "md5-")
	if !ok {
		return false
	}
	value, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(value) != md5.Size*2 {
		return false
	}
	digest, err := hex.DecodeString(string(value))
	return err == nil && len(digest) == md5.Size
}

func verifyIntegrity(h hash.Hash, expected []byte) error {
	if h != nil && subtle.ConstantTimeCompare(h.Sum(nil), expected) != 1 {
		return fmt.Errorf("installer integrity mismatch")
	}
	return nil
}
