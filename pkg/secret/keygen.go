// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/bborbe/errors"
)

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// DefaultKeyLength is the default length of generated keys.
const DefaultKeyLength = 8

//counterfeiter:generate -o ../../mocks/key-generator.go --fake-name KeyGenerator . KeyGenerator

// KeyGenerator produces fresh, URL-safe, short alphanumeric secret keys.
type KeyGenerator interface {
	// Generate returns a new random Key. The returned key is a non-empty,
	// URL-safe alphanumeric string; it makes no uniqueness guarantee against
	// the store (the store's Create enforces uniqueness via check-and-set).
	Generate(ctx context.Context) (Key, error)
}

// NewKeyGenerator returns a KeyGenerator producing keys of length keyLength
// from the base62 alphabet [A-Za-z0-9], drawing randomness from crypto/rand.
func NewKeyGenerator(keyLength int) KeyGenerator {
	return &keyGenerator{keyLength: keyLength}
}

type keyGenerator struct {
	keyLength int
}

func (g *keyGenerator) Generate(ctx context.Context) (Key, error) {
	result := make([]byte, g.keyLength)
	for i := 0; i < g.keyLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", errors.Wrapf(ctx, err, "read random bytes failed")
		}
		result[i] = alphabet[n.Int64()]
	}
	return Key(result), nil
}
