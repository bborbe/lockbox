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
	// Search returns a record (key, name, username, url) for every secret whose
	// key, name, username, url, or description contains query (case-insensitive).
	// An empty query matches every secret. No secret value is returned.
	Search(ctx context.Context, query string) (SearchRecords, error)
	// ReEncrypt rewrites every stored secret sealed under the current primary
	// key. It reads each secret through the configured crypter and Upserts it
	// back, so any secret still sealed under a non-primary (or pre-keyring) key
	// is re-sealed under the primary. It is idempotent (re-encrypting an
	// already-primary secret is harmless) and crash-safe (each secret is a
	// single-key overwrite; an interrupted run leaves the prior ciphertext
	// intact and still decryptable under the still-present keys; re-running
	// completes the conversion).
	ReEncrypt(ctx context.Context) error
}

// Keys is a list of secret keys.
type Keys []Key

// SearchRecord is one search match: the secret's Key plus the metadata
// fields (Name, Username, URL) needed to render a TeamVault search result.
// It deliberately carries no secret value (Password/File).
type SearchRecord struct {
	Key      Key
	Name     string
	Username string
	URL      string
}

// SearchRecords is a list of search matches.
type SearchRecords []SearchRecord

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

func (s *store) Search(ctx context.Context, query string) (SearchRecords, error) {
	needle := strings.ToLower(query)
	result := SearchRecords{}
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
			result = append(result, SearchRecord{
				Key:      key,
				Name:     secret.Name,
				Username: secret.Username,
				URL:      secret.URL,
			})
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
	}
	return result, nil
}

func (s *store) ReEncrypt(ctx context.Context) error {
	records, err := s.Search(ctx, "")
	if err != nil {
		return errors.Wrapf(ctx, err, "re-encrypt: list secrets failed")
	}
	for _, record := range records {
		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx, ctx.Err(), "re-encrypt cancelled")
		default:
		}
		sec, err := s.Get(ctx, record.Key)
		if err != nil {
			return errors.Wrapf(ctx, err, "re-encrypt: read secret %s failed", record.Key)
		}
		if err := s.Upsert(ctx, record.Key, *sec); err != nil {
			return errors.Wrapf(ctx, err, "re-encrypt: rewrite secret %s failed", record.Key)
		}
	}
	return nil
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
