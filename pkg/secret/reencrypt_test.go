// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret_test

import (
	"context"

	"github.com/bborbe/crypto"
	"github.com/bborbe/memorykv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/keyring"
	"github.com/bborbe/lockbox/pkg/secret"
)

// testPrimaryKey is a fixed 32-byte AES-256 primary key used only in tests.
var testPrimaryKey = crypto.SecretKey("11111111111111111111111111111111"[:32])

// testOldKey is a fixed 32-byte AES-256 retired key used only in tests.
var testOldKey = crypto.SecretKey("22222222222222222222222222222222"[:32])

// testLegacyKey is a fixed 32-byte AES-256 key used only in tests.
var testLegacyKey = crypto.SecretKey("33333333333333333333333333333333"[:32])

var _ = Describe("ReEncrypt", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// -------------------------------------------------------------------------
	// AC 9: Sweep moves every secret to the primary key
	// -------------------------------------------------------------------------
	Describe("sweep moves every secret to the primary key", func() {
		It("re-encrypts all secrets under the primary key", func() {
			db, err := memorykv.OpenMemory(ctx)
			Expect(err).To(BeNil())
			defer db.Close()

			// Seed 3 secrets using the old key.
			oldRing, err := keyring.New(ctx, testOldKey)
			Expect(err).To(BeNil())
			oldStore := secret.NewStore(db, oldRing)
			secrets := []secret.Secret{
				{Username: "alice", Password: "pw-alice"},
				{Username: "bob", Password: "pw-bob"},
				{Username: "carol", Password: "pw-carol"},
			}
			keys := []secret.Key{"key-1", "key-2", "key-3"}
			for i, k := range keys {
				Expect(oldStore.Upsert(ctx, k, secrets[i])).To(BeNil())
			}

			// Build a store with both keys; primary is first.
			dualRing, err := keyring.New(ctx, testPrimaryKey, testOldKey)
			Expect(err).To(BeNil())
			dualStore := secret.NewStore(db, dualRing)
			Expect(dualStore.ReEncrypt(ctx)).To(BeNil())

			// Verify with a primary-only ring — all secrets must decrypt.
			primaryRing, err := keyring.New(ctx, testPrimaryKey)
			Expect(err).To(BeNil())
			primaryOnly := secret.NewStore(db, primaryRing)
			for i, k := range keys {
				sec, err := primaryOnly.Get(ctx, k)
				Expect(err).To(BeNil())
				Expect(*sec).To(Equal(secrets[i]))
			}
		})
	})

	// -------------------------------------------------------------------------
	// AC 10: Sweep is idempotent / re-runnable
	// -------------------------------------------------------------------------
	Describe("sweep is idempotent", func() {
		It("re-running ReEncrypt twice leaves all secrets unchanged", func() {
			db, err := memorykv.OpenMemory(ctx)
			Expect(err).To(BeNil())
			defer db.Close()

			// Seed secrets under the old key.
			oldRing, err := keyring.New(ctx, testOldKey)
			Expect(err).To(BeNil())
			oldStore := secret.NewStore(db, oldRing)
			original := secret.Secret{Username: "dave", Password: "pw-dave"}
			Expect(oldStore.Upsert(ctx, secret.Key("idempotent-key"), original)).To(BeNil())

			// Build a store with both keys and run ReEncrypt twice.
			dualRing, err := keyring.New(ctx, testPrimaryKey, testOldKey)
			Expect(err).To(BeNil())
			dualStore := secret.NewStore(db, dualRing)
			Expect(dualStore.ReEncrypt(ctx)).To(BeNil())
			Expect(dualStore.ReEncrypt(ctx)).To(BeNil()) // second run must not error

			// Verify with primary-only ring.
			primaryRing, err := keyring.New(ctx, testPrimaryKey)
			Expect(err).To(BeNil())
			primaryOnly := secret.NewStore(db, primaryRing)
			sec, err := primaryOnly.Get(ctx, secret.Key("idempotent-key"))
			Expect(err).To(BeNil())
			Expect(*sec).To(Equal(original))
		})
	})

	// -------------------------------------------------------------------------
	// Legacy blobs are swept too
	// -------------------------------------------------------------------------
	Describe("legacy un-framed blobs", func() {
		It("re-encrypts a secret sealed with a raw pre-keyring Crypter", func() {
			db, err := memorykv.OpenMemory(ctx)
			Expect(err).To(BeNil())
			defer db.Close()

			// Seed a secret using the raw (non-keyring) crypter — produces an
			// un-framed ciphertext with no key-id prefix.
			rawStore := secret.NewStore(db, crypto.NewCrypter(testLegacyKey))
			legacySecret := secret.Secret{Username: "eve", Password: "pw-eve"}
			Expect(rawStore.Upsert(ctx, secret.Key("legacy-key"), legacySecret)).To(BeNil())

			// Build a store with the legacy key as a non-primary ring entry.
			dualRing, err := keyring.New(ctx, testPrimaryKey, testLegacyKey)
			Expect(err).To(BeNil())
			dualStore := secret.NewStore(db, dualRing)
			Expect(dualStore.ReEncrypt(ctx)).To(BeNil())

			// Verify with a primary-only ring — must decrypt under primary only.
			primaryRing, err := keyring.New(ctx, testPrimaryKey)
			Expect(err).To(BeNil())
			primaryOnly := secret.NewStore(db, primaryRing)
			sec, err := primaryOnly.Get(ctx, secret.Key("legacy-key"))
			Expect(err).To(BeNil())
			Expect(*sec).To(Equal(legacySecret))
		})
	})
})
