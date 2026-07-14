// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keyring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	stderrors "errors"
	"strings"

	"github.com/bborbe/crypto"
	"github.com/bborbe/errors"
)

// errKeyNotFound is a sentinel used internally to signal that no configured
// key could authenticate a ciphertext during the try-each loop.
var errKeyNotFound = stderrors.New("key not found")

// keyIDLen is the length of the truncated SHA-256 fingerprint used as a
// non-secret key identifier in the frame header.
const keyIDLen = 4

// frameVersion is the version byte prepended to every framed ciphertext.
const frameVersion byte = 0x01

// frameMagic is the ASCII bytes "LKB1" — a self-describing non-secret marker
// that identifies a keyring-framed ciphertext at rest.
var frameMagic = []byte{0x4C, 0x4B, 0x42, 0x31}

// FrameHeaderLen is the total length of the fixed prefix: frameMagic (4) +
// frameVersion (1) + keyID (keyIDLen).
var FrameHeaderLen = len(frameMagic) + 1 + keyIDLen

// GcmMinLen is the minimum byte length of a valid AES-GCM ciphertext (12-byte
// nonce minimum; shorter values can never authenticate and are skipped).
const GcmMinLen = 12

// counterfeiter:generate -o ../../mocks/keyring.go --fake-name Keyring . Keyring

// Keyring is an ordered set of AES keys satisfying crypto.Crypter. The first
// key is the primary: Encrypt always seals under it. Decrypt resolves any
// secret sealed under any configured key (or a pre-keyring un-framed blob).
type Keyring interface {
	crypto.Crypter
}

// Parse builds a Keyring from the two mutually-exclusive key configuration
// sources: single (LOCKBOX_ENCRYPTION_KEY, one base64 key) and list
// (LOCKBOX_ENCRYPTION_KEYS, comma-separated base64 keys, primary first).
// Exactly one of single/list must be non-empty; every key must base64-decode
// to 16 or 32 bytes and be distinct, or Parse returns a wrapped error.
// Duplicate detection and empty-ring rejection are delegated to New.
func Parse(ctx context.Context, single string, list string) (Keyring, error) {
	single = strings.TrimSpace(single)
	list = strings.TrimSpace(list)

	// Exactly-one rule.
	if single == "" && list == "" {
		return nil, errors.New(
			ctx,
			"either LOCKBOX_ENCRYPTION_KEY or LOCKBOX_ENCRYPTION_KEYS must be set",
		)
	}
	if single != "" && list != "" {
		return nil, errors.New(
			ctx,
			"LOCKBOX_ENCRYPTION_KEY and LOCKBOX_ENCRYPTION_KEYS are mutually exclusive; set exactly one",
		)
	}

	// Collect base64 entries.
	var entries []string
	if list != "" {
		parts := strings.Split(list, ",")
		for _, p := range parts {
			entries = append(entries, strings.TrimSpace(p))
		}
	} else {
		entries = []string{single}
	}

	// Reject empty/whitespace entries.
	for i, e := range entries {
		if e == "" {
			return nil, errors.Errorf(ctx, "LOCKBOX_ENCRYPTION_KEYS entry %d is empty", i)
		}
	}

	// Decode and validate each key.
	keys := make([]crypto.SecretKey, 0, len(entries))
	for i, entry := range entries {
		raw, err := base64.StdEncoding.DecodeString(entry)
		if err != nil {
			return nil, errors.Wrapf(
				ctx,
				err,
				"LOCKBOX_ENCRYPTION_KEYS entry %d: base64 decode failed",
				i,
			)
		}
		if len(raw) != 16 && len(raw) != 32 {
			return nil, errors.Errorf(
				ctx,
				"LOCKBOX_ENCRYPTION_KEYS entry %d: must decode to 16 or 32 bytes, got %d",
				i,
				len(raw),
			)
		}
		keys = append(keys, crypto.SecretKey(raw))
	}

	// Build keyring (it rejects duplicates and empty input).
	ring, err := New(ctx, keys...)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "build keyring failed")
	}
	return ring, nil
}

// New returns a Keyring holding the given keys in the order provided. The
// first key is the primary and is used for all Encrypt operations. The list
// must contain at least one key, each key must be 16 or 32 bytes, and no two
// keys may be equal or share the same content-derived identifier.
func New(ctx context.Context, keys ...crypto.SecretKey) (Keyring, error) {
	if len(keys) == 0 {
		return nil, errors.New(ctx, "keyring requires at least one key")
	}

	entries := make([]entry, 0, len(keys))
	seenIDs := make(map[string]struct{}, len(keys))
	seenBytes := make(map[string]struct{}, len(keys))

	for i, key := range keys {
		keyBytes := key.Bytes()
		if len(keyBytes) != 16 && len(keyBytes) != 32 {
			return nil, errors.Errorf(
				ctx,
				"keyring key %d: invalid length %d (want 16 or 32)",
				i,
				len(keyBytes),
			)
		}
		if _, ok := seenBytes[string(keyBytes)]; ok {
			return nil, errors.Errorf(ctx, "keyring key %d: duplicate key", i)
		}
		seenBytes[string(keyBytes)] = struct{}{}

		id := keyID(key)
		if _, ok := seenIDs[string(id)]; ok {
			return nil, errors.Errorf(ctx, "keyring key %d: duplicate key-id", i)
		}
		seenIDs[string(id)] = struct{}{}

		entries = append(entries, entry{
			id:      id,
			crypter: crypto.NewCrypter(key),
		})
	}

	return &keyring{entries: entries}, nil
}

type entry struct {
	id      []byte
	crypter crypto.Crypter
}

type keyring struct {
	entries []entry
}

// keyID returns a short, deterministic fingerprint of the key bytes computed
// as the first keyIDLen bytes of SHA-256. The identifier is non-secret and
// must not leak the key material.
func keyID(key crypto.SecretKey) []byte {
	sum := sha256.Sum256(key.Bytes())
	id := make([]byte, keyIDLen)
	copy(id, sum[:keyIDLen])
	return id
}

// buildFrame assembles a keyring frame from the given id and ciphertext blob.
func buildFrame(id []byte, blob []byte) []byte {
	result := make([]byte, 0, FrameHeaderLen+len(blob))
	result = append(result, frameMagic...)
	result = append(result, frameVersion)
	result = append(result, id...)
	result = append(result, blob...)
	return result
}

// parseFrame extracts the key-id and ciphertext blob from a framed value.
// It returns ok==false when the value is shorter than the fixed header, the
// magic bytes do not match, or the version byte is not recognised. It never
// panics and never reads out of bounds.
func parseFrame(value []byte) (id []byte, blob []byte, ok bool) {
	if len(value) < FrameHeaderLen {
		return nil, nil, false
	}
	if !bytes.Equal(value[:len(frameMagic)], frameMagic) {
		return nil, nil, false
	}
	if value[len(frameMagic)] != frameVersion {
		return nil, nil, false
	}
	offset := len(frameMagic) + 1
	idStart := offset
	idEnd := idStart + keyIDLen
	id = value[idStart:idEnd:idEnd]
	blob = value[idEnd:]
	return id, blob, true
}

// Encrypt seals the value under the primary key and prepends a frame that
// identifies which key was used so that Decrypt can select the correct key
// without trying them all.
func (k *keyring) Encrypt(ctx context.Context, value []byte) ([]byte, error) {
	blob, err := k.entries[0].crypter.Encrypt(ctx, value)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "keyring encrypt failed")
	}
	return buildFrame(k.entries[0].id, blob), nil
}

// tryKey attempts to decrypt blob using the given entry. It returns errKeyNotFound
// when the ciphertext is too short or authentication fails.
func tryKey(ctx context.Context, e entry, blob []byte) ([]byte, error) {
	if len(blob) < GcmMinLen {
		return nil, errKeyNotFound
	}
	plaintext, err := e.crypter.Decrypt(ctx, blob)
	if err != nil {
		return nil, errKeyNotFound
	}
	return plaintext, nil
}

// tryByID attempts to decrypt a framed blob using the entry whose id matches
// the frame's key-id. It returns nil, errKeyNotFound when the id is unknown or
// authentication fails for the matching entry.
func (k *keyring) tryByID(ctx context.Context, blob []byte, frameID []byte) ([]byte, error) {
	for _, e := range k.entries {
		if bytes.Equal(e.id, frameID) {
			return tryKey(ctx, e, blob)
		}
	}
	return nil, errKeyNotFound
}

// tryAllKeys attempts to decrypt blob using every entry in order. It returns
// the first successful plaintext or errKeyNotFound if no key authenticates.
func (k *keyring) tryAllKeys(ctx context.Context, blob []byte) ([]byte, error) {
	for _, e := range k.entries {
		plaintext, err := tryKey(ctx, e, blob)
		if err == nil {
			return plaintext, nil
		}
	}
	return nil, errKeyNotFound
}

// Decrypt attempts to recover the plaintext using the following ordered
// precedence:
//
//  1. If the value starts with a valid keyring frame, try the keyed entry
//     matching the frame's key-id.
//  2. If step 1 found a matching id but authentication failed, try every
//     configured key in order against the blob.
//  3. Try every configured key against the original un-framed value (legacy
//     pre-keyring ciphertext).
//
// Discrimination between candidates is always by AES-GCM authentication
// (Decrypt returning no error), never by the frame marker alone. A too-short
// candidate is silently skipped rather than panicking.
func (k *keyring) Decrypt(ctx context.Context, value []byte) ([]byte, error) {
	if plaintext, ok := k.tryFramed(ctx, value); ok {
		return plaintext, nil
	}

	// Step 3: legacy un-framed — try every key against the raw value.
	plaintext, err := k.tryAllKeys(ctx, value)
	if err == nil {
		return plaintext, nil
	}

	return nil, errors.Errorf(
		ctx,
		"keyring decrypt failed: no configured key authenticates the ciphertext",
	)
}

// tryFramed attempts to decrypt a framed value. It returns (plaintext, true)
// on success or (nil, false) when the value does not carry a valid frame.
func (k *keyring) tryFramed(ctx context.Context, value []byte) ([]byte, bool) {
	if len(value) < FrameHeaderLen {
		return nil, false
	}
	id, blob, ok := parseFrame(value)
	if !ok {
		return nil, false
	}

	plaintext, err := k.tryByID(ctx, blob, id)
	if err == nil {
		return plaintext, true
	}

	// Step 2: explicit id didn't work — try every key against the blob.
	plaintext, err = k.tryAllKeys(ctx, blob)
	if err == nil {
		return plaintext, true
	}

	return nil, false
}
