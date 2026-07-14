// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"

	"github.com/bborbe/crypto"
	"github.com/bborbe/errors"
	libkv "github.com/bborbe/kv"
)

// ErrKeyExists is returned by Create when the target key is already present.
var ErrKeyExists = stderrors.New("key already exists")

// bucketName is the kv bucket Lockbox stores secrets in.
var bucketName = libkv.NewBucketName("secrets")

//counterfeiter:generate -o ../../mocks/secret-store.go --fake-name SecretStore . Store

// Store persists secrets by Key and supports lookup and substring search.
type Store interface {
	// Upsert creates or replaces the secret stored under key.
	Upsert(ctx context.Context, key Key, secret Secret) error
	// Create stores secret under key only if key does not already exist.
	// If key is already present it returns ErrKeyExists and does not modify
	// the stored secret.
	//
	// Create is check-and-set at the store level; collision at Lockbox scale
	// is astronomically unlikely. Callers should regenerate the key and retry
	// on ErrKeyExists.
	Create(ctx context.Context, key Key, secret Secret) error
	// Get returns the secret stored under key, or an error if absent.
	Get(ctx context.Context, key Key) (*Secret, error)
	// Search returns the keys whose key or username contains query
	// (case-insensitive). An empty query matches every secret.
	Search(ctx context.Context, query string) (Keys, error)
}

// Keys is a list of secret keys.
type Keys []Key

// NewStore returns a Store backed by the given kv database. Every secret is
// JSON-marshalled and encrypted with crypter before it is written, and
// decrypted on read, so only ciphertext ever touches the underlying kv
// database.
func NewStore(db libkv.DB, crypter crypto.Crypter) Store {
	return &store{
		kv:      libkv.NewStore[Key, []byte](db, bucketName),
		crypter: crypter,
	}
}

type store struct {
	kv      libkv.Store[Key, []byte]
	crypter crypto.Crypter
}

func (s *store) Upsert(ctx context.Context, key Key, secret Secret) error {
	data, err := json.Marshal(
		secret,
	) // #nosec G117 -- marshalled only to be immediately encrypted below
	if err != nil {
		return errors.Wrapf(ctx, err, "marshal secret %s failed", key)
	}
	encrypted, err := s.crypter.Encrypt(ctx, data)
	if err != nil {
		return errors.Wrapf(ctx, err, "encrypt secret %s failed", key)
	}
	if err := s.kv.Add(ctx, key, encrypted); err != nil {
		return errors.Wrapf(ctx, err, "upsert secret %s failed", key)
	}
	return nil
}

func (s *store) Create(ctx context.Context, key Key, secret Secret) error {
	exists, err := s.kv.Exists(ctx, key)
	if err != nil {
		return errors.Wrapf(ctx, err, "create secret %s failed", key)
	}
	if exists {
		return errors.Wrapf(ctx, ErrKeyExists, "create secret %s failed", key)
	}
	data, err := json.Marshal(
		secret,
	) // #nosec G117 -- marshalled only to be immediately encrypted below
	if err != nil {
		return errors.Wrapf(ctx, err, "marshal secret %s failed", key)
	}
	encrypted, err := s.crypter.Encrypt(ctx, data)
	if err != nil {
		return errors.Wrapf(ctx, err, "encrypt secret %s failed", key)
	}
	if err := s.kv.Add(ctx, key, encrypted); err != nil {
		return errors.Wrapf(ctx, err, "create secret %s failed", key)
	}
	return nil
}

func (s *store) Get(ctx context.Context, key Key) (*Secret, error) {
	encrypted, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "get secret %s failed", key)
	}
	secret, err := s.decrypt(ctx, *encrypted)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "decrypt secret %s failed", key)
	}
	return secret, nil
}

func (s *store) Search(ctx context.Context, query string) (Keys, error) {
	needle := strings.ToLower(query)
	result := Keys{}
	err := s.kv.Map(ctx, func(ctx context.Context, key Key, encrypted []byte) error {
		secret, err := s.decrypt(ctx, encrypted)
		if err != nil {
			return errors.Wrapf(ctx, err, "decrypt secret %s failed", key)
		}
		if needle == "" ||
			strings.Contains(strings.ToLower(key.String()), needle) ||
			strings.Contains(strings.ToLower(secret.Name), needle) ||
			strings.Contains(strings.ToLower(secret.Username), needle) ||
			strings.Contains(strings.ToLower(secret.URL), needle) ||
			strings.Contains(strings.ToLower(secret.Description), needle) {
			result = append(result, key)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
	}
	return result, nil
}

// decrypt decrypts and unmarshals a stored ciphertext into a Secret.
func (s *store) decrypt(ctx context.Context, encrypted []byte) (*Secret, error) {
	data, err := s.crypter.Decrypt(ctx, encrypted)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "decrypt failed")
	}
	var secret Secret
	if err := json.Unmarshal(data, &secret); err != nil {
		return nil, errors.Wrapf(ctx, err, "unmarshal secret failed")
	}
	return &secret, nil
}
