// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret

import (
	"context"
	"strings"

	"github.com/bborbe/errors"
	libkv "github.com/bborbe/kv"
)

// bucketName is the kv bucket Lockbox stores secrets in.
var bucketName = libkv.NewBucketName("secrets")

//counterfeiter:generate -o ../../mocks/secret-store.go --fake-name SecretStore . Store

// Store persists secrets by Key and supports lookup and substring search.
type Store interface {
	// Upsert creates or replaces the secret stored under key.
	Upsert(ctx context.Context, key Key, secret Secret) error
	// Get returns the secret stored under key, or an error if absent.
	Get(ctx context.Context, key Key) (*Secret, error)
	// Search returns the keys whose key or username contains query
	// (case-insensitive). An empty query matches every secret.
	Search(ctx context.Context, query string) (Keys, error)
}

// Keys is a list of secret keys.
type Keys []Key

// NewStore returns a Store backed by the given kv database.
func NewStore(db libkv.DB) Store {
	return &store{
		kv: libkv.NewStore[Key, Secret](db, bucketName),
	}
}

type store struct {
	kv libkv.Store[Key, Secret]
}

func (s *store) Upsert(ctx context.Context, key Key, secret Secret) error {
	if err := s.kv.Add(ctx, key, secret); err != nil {
		return errors.Wrapf(ctx, err, "upsert secret %s failed", key)
	}
	return nil
}

func (s *store) Get(ctx context.Context, key Key) (*Secret, error) {
	secret, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "get secret %s failed", key)
	}
	return secret, nil
}

func (s *store) Search(ctx context.Context, query string) (Keys, error) {
	needle := strings.ToLower(query)
	result := Keys{}
	err := s.kv.Map(ctx, func(ctx context.Context, key Key, secret Secret) error {
		if needle == "" ||
			strings.Contains(strings.ToLower(key.String()), needle) ||
			strings.Contains(strings.ToLower(secret.Username), needle) {
			result = append(result, key)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
	}
	return result, nil
}
