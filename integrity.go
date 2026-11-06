package main

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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
	default:
		return nil, nil, fmt.Errorf("unsupported installer integrity algorithm")
	}
	expected, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(expected) != h.Size() {
		return nil, nil, fmt.Errorf("invalid installer integrity")
	}
	return h, expected, nil
}

func verifyIntegrity(h hash.Hash, expected []byte) error {
	if h != nil && subtle.ConstantTimeCompare(h.Sum(nil), expected) != 1 {
		return fmt.Errorf("installer integrity mismatch")
	}
	return nil
}
