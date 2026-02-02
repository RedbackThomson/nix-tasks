package nix

import (
	"encoding/hex"

	"github.com/nix-community/go-nix/pkg/storepath"
)

// ParseStorePath validates and parses a Nix store path
func ParseStorePath(path string) (*storepath.StorePath, error) {
	return storepath.FromAbsolutePath(path)
}

// StorePathHash extracts the hash component from a store path
func StorePathHash(path string) (string, error) {
	sp, err := ParseStorePath(path)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sp.Digest), nil
}
