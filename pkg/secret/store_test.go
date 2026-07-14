// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret_test

import (
	"context"
	"strings"

	"github.com/bborbe/crypto"
	libkv "github.com/bborbe/kv"
	"github.com/bborbe/memorykv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/secret"
)

// testEncryptionKey is a fixed 32-byte AES-256 key used only in tests.
var testEncryptionKey = crypto.SecretKey("01234567890123456789012345678901"[:32])

var _ = Describe("Store", func() {
	var ctx context.Context
	var db libkv.DB
	var store secret.Store

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		db, err = memorykv.OpenMemory(ctx)
		Expect(err).To(BeNil())
		store = secret.NewStore(db, crypto.NewCrypter(testEncryptionKey))
	})

	Describe("Upsert + Get", func() {
		It("stores and returns the secret", func() {
			value := secret.Secret{
				Username: "alice",
				URL:      "https://example.com",
				Password: "s3cr3t",
				File:     "ZmlsZQ==",
			}
			err := store.Upsert(ctx, secret.Key("AbC123"), value)
			Expect(err).To(BeNil())

			found, err := store.Get(ctx, secret.Key("AbC123"))
			Expect(err).To(BeNil())
			Expect(found).NotTo(BeNil())
			Expect(*found).To(Equal(value))
		})

		It("replaces an existing secret on a second Upsert", func() {
			key := secret.Key("AbC123")
			Expect(store.Upsert(ctx, key, secret.Secret{Username: "alice"})).To(BeNil())
			Expect(store.Upsert(ctx, key, secret.Secret{Username: "bob"})).To(BeNil())

			found, err := store.Get(ctx, key)
			Expect(err).To(BeNil())
			Expect(found.Username).To(Equal("bob"))
		})

		It("returns an error for a missing key", func() {
			_, err := store.Get(ctx, secret.Key("missing"))
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("Search", func() {
		BeforeEach(func() {
			Expect(
				store.Upsert(ctx, secret.Key("GitHubToken"), secret.Secret{Username: "octocat"}),
			).To(BeNil())
			Expect(
				store.Upsert(ctx, secret.Key("AwsKey"), secret.Secret{Username: "root"}),
			).To(BeNil())
			Expect(
				store.Upsert(ctx, secret.Key("DbPassword"), secret.Secret{Username: "admin"}),
			).To(BeNil())
		})

		It("returns every key for an empty query", func() {
			keys, err := store.Search(ctx, "")
			Expect(err).To(BeNil())
			Expect(keys).To(HaveLen(3))
		})

		It("matches a substring of the key case-insensitively", func() {
			keys, err := store.Search(ctx, "github")
			Expect(err).To(BeNil())
			Expect(keys).To(ConsistOf(secret.Key("GitHubToken")))
		})

		It("matches a substring of the username case-insensitively", func() {
			keys, err := store.Search(ctx, "OCTOCAT")
			Expect(err).To(BeNil())
			Expect(keys).To(ConsistOf(secret.Key("GitHubToken")))
		})

		It("returns no keys when nothing matches", func() {
			keys, err := store.Search(ctx, "nope")
			Expect(err).To(BeNil())
			Expect(keys).To(BeEmpty())
		})
	})

	Describe("Encryption at rest", func() {
		It("round-trips the secret through Upsert and Get unchanged", func() {
			value := secret.Secret{
				Username: "alice",
				URL:      "https://example.com",
				Password: "s3cr3t-password",
				File:     "ZmlsZQ==",
			}
			Expect(store.Upsert(ctx, secret.Key("RoundTrip"), value)).To(BeNil())

			found, err := store.Get(ctx, secret.Key("RoundTrip"))
			Expect(err).To(BeNil())
			Expect(*found).To(Equal(value))
		})

		It("never writes the plaintext password to the underlying kv bucket", func() {
			password := "s3cr3t-password-not-in-ciphertext"
			Expect(
				store.Upsert(ctx, secret.Key("LeakCheck"), secret.Secret{
					Username: "alice",
					Password: password,
				}),
			).To(BeNil())

			// Read the raw bytes stored in the "secrets" bucket directly, bypassing
			// the Store's decryption, to verify only ciphertext is persisted.
			rawStore := libkv.NewStore[secret.Key, []byte](db, libkv.NewBucketName("secrets"))
			raw, err := rawStore.Get(ctx, secret.Key("LeakCheck"))
			Expect(err).To(BeNil())
			Expect(raw).NotTo(BeNil())
			Expect(strings.Contains(string(*raw), password)).To(BeFalse())
		})
	})
})
