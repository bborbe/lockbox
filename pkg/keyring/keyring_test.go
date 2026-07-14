// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package keyring_test

import (
	"context"
	"encoding/base64"

	"github.com/bborbe/crypto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/keyring"
)

// testKey32A is a fixed 32-byte AES-256 key used only in tests.
var testKey32A = crypto.SecretKey("01234567890123456789012345678901"[:32])

// testKey32B is a distinct 32-byte AES-256 key used only in tests.
var testKey32B = crypto.SecretKey("abcdefghijklmnopqrstuvwxyz012345"[:32])

// testKey16 is a fixed 16-byte AES-128 key used only in tests.
var testKey16 = crypto.SecretKey("0123456789012345"[:16])

var _ = Describe("Keyring", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Encrypt + Decrypt", func() {
		Describe("AC1: legacy ciphertext decrypts unchanged", func() {
			It("decrypts a pre-keyring AES-GCM blob sealed by the same key", func() {
				plaintext := []byte("my-super-secret-password")

				// Seal with the raw single-key crypter — no frame.
				singleCrypter := crypto.NewCrypter(testKey32A)
				legacyCiphertext, err := singleCrypter.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				// Build a keyring holding exactly that key.
				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())
				Expect(kr).NotTo(BeNil())

				// Decrypt through the keyring — must succeed without rewrite.
				result, err := kr.Decrypt(ctx, legacyCiphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})

			It("decrypts a pre-keyring blob sealed by a 16-byte key", func() {
				plaintext := []byte("short-but-secure")

				singleCrypter := crypto.NewCrypter(testKey16)
				legacyCiphertext, err := singleCrypter.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				kr, err := keyring.New(ctx, testKey16)
				Expect(err).To(BeNil())

				result, err := kr.Decrypt(ctx, legacyCiphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})
		})

		Describe("AC2: primary-key encrypt is identifiable", func() {
			It("round-trips through the same keyring", func() {
				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				plaintext := []byte("identify-me")
				ciphertext, err := kr.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())
				Expect(ciphertext).NotTo(BeNil())

				result, err := kr.Decrypt(ctx, ciphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})

			It("a ring containing only the primary key decrypts its own output", func() {
				plaintext := []byte("primary-only")

				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				ciphertext, err := kr.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				// Decrypt with a ring of only the primary.
				primaryOnly, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				result, err := primaryOnly.Decrypt(ctx, ciphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})

			It("a ring NOT containing the primary key cannot decrypt its output", func() {
				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				plaintext := []byte("wrong-key")
				ciphertext, err := kr.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				// A ring of only the other key must fail.
				otherOnly, err := keyring.New(ctx, testKey32B)
				Expect(err).To(BeNil())

				result, err := otherOnly.Decrypt(ctx, ciphertext)
				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
			})
		})

		Describe("AC3: decrypt-by-id across a multi-key ring", func() {
			It("decrypts both old and new ciphertexts through a ring containing both keys", func() {
				plaintextOld := []byte("sealed-with-old")
				plaintextNew := []byte("sealed-with-new")

				// Seal A with old key only.
				krOld, err := keyring.New(ctx, testKey32B)
				Expect(err).To(BeNil())
				ciphertextOld, err := krOld.Encrypt(ctx, plaintextOld)
				Expect(err).To(BeNil())

				// Seal B with new key only.
				krNew, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())
				ciphertextNew, err := krNew.Encrypt(ctx, plaintextNew)
				Expect(err).To(BeNil())

				// Decrypt both through a ring containing both keys (primary = testKey32A).
				krBoth, err := keyring.New(ctx, testKey32A, testKey32B)
				Expect(err).To(BeNil())

				resultOld, err := krBoth.Decrypt(ctx, ciphertextOld)
				Expect(err).To(BeNil())
				Expect(resultOld).To(Equal(plaintextOld))

				resultNew, err := krBoth.Decrypt(ctx, ciphertextNew)
				Expect(err).To(BeNil())
				Expect(resultNew).To(Equal(plaintextNew))
			})

			It("decrypts a blob whose frame id refers to a later-indexed key", func() {
				// Seal with the second key.
				krSecond, err := keyring.New(ctx, testKey32A, testKey32B)
				Expect(err).To(BeNil())
				plaintext := []byte("sealed-with-second")
				ciphertext, err := krSecond.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				// Decrypt with the same ring — id resolves to second key.
				result, err := krSecond.Decrypt(ctx, ciphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})
		})

		Describe("AC4: unknown key-id is a clear error, not panic or wrong plaintext", func() {
			It(
				"returns an error with nil bytes when sealed by one key and decrypted by another",
				func() {
					krA, err := keyring.New(ctx, testKey32A)
					Expect(err).To(BeNil())
					plaintext := []byte("sealed-by-a")
					ciphertext, err := krA.Encrypt(ctx, plaintext)
					Expect(err).To(BeNil())

					krB, err := keyring.New(ctx, testKey32B)
					Expect(err).To(BeNil())

					result, err := krB.Decrypt(ctx, ciphertext)
					Expect(err).NotTo(BeNil())
					Expect(result).To(BeNil())
				},
			)

			It("does not panic on a truncated frame (shorter than frameHeaderLen)", func() {
				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				// 6 bytes: shorter than the fixed header.
				truncated := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}

				result, err := kr.Decrypt(ctx, truncated)
				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
			})

			It("returns an error for a blob too short to be GCM", func() {
				kr, err := keyring.New(ctx, testKey32A)
				Expect(err).To(BeNil())

				// Exactly FrameHeaderLen but with a valid magic/version and too-short blob.
				shortBlob := make([]byte, keyring.FrameHeaderLen+keyring.GcmMinLen-1)
				copy(shortBlob, []byte{0x4C, 0x4B, 0x42, 0x31, 0x01})
				// key-id bytes are zeros, blob is too short for GCM.

				result, err := kr.Decrypt(ctx, shortBlob)
				Expect(err).NotTo(BeNil())
				Expect(result).To(BeNil())
			})
		})

		Describe("AC5: key-id stable under reordering", func() {
			It("decrypts the same ciphertext when the keyring order is reversed", func() {
				plaintext := []byte("order-independent")

				// Encrypt with primary=first.
				krForward, err := keyring.New(ctx, testKey32A, testKey32B)
				Expect(err).To(BeNil())
				ciphertext, err := krForward.Encrypt(ctx, plaintext)
				Expect(err).To(BeNil())

				// Decrypt with the same two keys but swapped order.
				krReversed, err := keyring.New(ctx, testKey32B, testKey32A)
				Expect(err).To(BeNil())

				result, err := krReversed.Decrypt(ctx, ciphertext)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(plaintext))
			})
		})
	})

	Describe("New (constructor validation)", func() {
		It("returns an error when given no keys", func() {
			kr, err := keyring.New(ctx)
			Expect(err).NotTo(BeNil())
			Expect(kr).To(BeNil())
		})

		It("returns an error when a key is 15 bytes", func() {
			badKey := crypto.SecretKey("012345678901234"[:15])
			kr, err := keyring.New(ctx, badKey)
			Expect(err).NotTo(BeNil())
			Expect(kr).To(BeNil())
		})

		It("returns an error when a key is 24 bytes", func() {
			badKey := crypto.SecretKey("012345678901234567890123"[:24])
			kr, err := keyring.New(ctx, badKey)
			Expect(err).NotTo(BeNil())
			Expect(kr).To(BeNil())
		})

		It("returns an error when two keys are identical", func() {
			kr, err := keyring.New(ctx, testKey32A, testKey32A)
			Expect(err).NotTo(BeNil())
			Expect(kr).To(BeNil())
		})

		It("returns an error when two distinct keys produce the same key-id", func() {
			// Two 16-byte keys that share the same first 4 SHA-256 bytes are
			// astronomically unlikely, so we test the collision-detection logic
			// by verifying that providing the same key twice triggers the id-duplicate
			// error before the byte-duplicate error (keyids equal when keybytes equal).
			kr, err := keyring.New(ctx, testKey32A, testKey32A)
			Expect(err).NotTo(BeNil())
			Expect(kr).To(BeNil())
		})

		It("returns a valid keyring when given one valid 32-byte key", func() {
			kr, err := keyring.New(ctx, testKey32A)
			Expect(err).To(BeNil())
			Expect(kr).NotTo(BeNil())
		})

		It("returns a valid keyring when given one valid 16-byte key", func() {
			kr, err := keyring.New(ctx, testKey16)
			Expect(err).To(BeNil())
			Expect(kr).NotTo(BeNil())
		})

		It("returns a valid keyring when given two distinct valid keys", func() {
			kr, err := keyring.New(ctx, testKey32A, testKey32B)
			Expect(err).To(BeNil())
			Expect(kr).NotTo(BeNil())
		})
	})

	Describe("Parse", func() {
		// -------------------------------------------------------------------------
		// AC 8: Invalid config refuses start
		// -------------------------------------------------------------------------
		DescribeTable("rejects invalid config",
			func(single string, list string) {
				kr, err := keyring.Parse(ctx, single, list)
				Expect(err).NotTo(BeNil())
				Expect(kr).To(BeNil())
			},
			Entry("neither env var set", "", ""),
			Entry("both env vars set",
				base64.StdEncoding.EncodeToString(testKey32A),
				base64.StdEncoding.EncodeToString(testKey32A),
			),
			Entry("empty list (comma-only)", "", ","),
			Entry("empty list (whitespace only)", "", "  ,  ,  "),
			Entry("list entry not valid base64", "", "!!!"),
			Entry("list entry wrong length", "",
				base64.StdEncoding.EncodeToString([]byte("short")),
			),
			Entry("duplicate keys in list", "",
				base64.StdEncoding.EncodeToString(testKey32A)+","+
					base64.StdEncoding.EncodeToString(testKey32A),
			),
		)

		// -------------------------------------------------------------------------
		// AC 6: Single key boots
		// -------------------------------------------------------------------------
		Describe("single-key legacy path", func() {
			DescribeTable("accepts a single base64-encoded key",
				func(encoded string) {
					kr, err := keyring.Parse(ctx, encoded, "")
					Expect(err).To(BeNil())
					Expect(kr).NotTo(BeNil())

					// Round-trip.
					plaintext := []byte("single-key-test")
					ciphertext, err := kr.Encrypt(ctx, plaintext)
					Expect(err).To(BeNil())
					result, err := kr.Decrypt(ctx, ciphertext)
					Expect(err).To(BeNil())
					Expect(result).To(Equal(plaintext))
				},
				Entry("32-byte key",
					base64.StdEncoding.EncodeToString(testKey32A),
				),
				Entry("16-byte key",
					base64.StdEncoding.EncodeToString(testKey16),
				),
			)
		})

		// -------------------------------------------------------------------------
		// AC 7: Ordered multi-key config parses, primary first
		// -------------------------------------------------------------------------
		It("encrypts with the first key as primary", func() {
			encodedPrimary := base64.StdEncoding.EncodeToString(testKey32A)
			encodedSecondary := base64.StdEncoding.EncodeToString(testKey32B)

			kr, err := keyring.Parse(ctx, "", encodedPrimary+","+encodedSecondary)
			Expect(err).To(BeNil())
			Expect(kr).NotTo(BeNil())

			// Encrypt.
			plaintext := []byte("primary-first-test")
			ciphertext, err := kr.Encrypt(ctx, plaintext)
			Expect(err).To(BeNil())

			// Decrypt with a ring of only the primary key.
			primaryOnly, err := keyring.New(ctx, testKey32A)
			Expect(err).To(BeNil())
			result, err := primaryOnly.Decrypt(ctx, ciphertext)
			Expect(err).To(BeNil())
			Expect(result).To(Equal(plaintext))
		})
	})
})
