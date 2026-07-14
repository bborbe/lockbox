// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secret_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/secret"
)

var _ = Describe("KeyGenerator", func() {
	var ctx context.Context
	var keyGen secret.KeyGenerator

	BeforeEach(func() {
		ctx = context.Background()
		keyGen = secret.NewKeyGenerator(secret.DefaultKeyLength)
	})

	Describe("Generate", func() {
		It("returns a non-empty key", func() {
			key, err := keyGen.Generate(ctx)
			Expect(err).To(BeNil())
			Expect(key).NotTo(BeEmpty())
		})

		It("returns a key whose length equals DefaultKeyLength", func() {
			key, err := keyGen.Generate(ctx)
			Expect(err).To(BeNil())
			Expect(len(key)).To(Equal(secret.DefaultKeyLength))
		})

		It("returns a key whose characters are all alphanumeric", func() {
			key, err := keyGen.Generate(ctx)
			Expect(err).To(BeNil())
			Expect(string(key)).To(MatchRegexp(`^[A-Za-z0-9]+$`))
		})

		It("returns different keys on successive calls", func() {
			key1, err := keyGen.Generate(ctx)
			Expect(err).To(BeNil())
			key2, err := keyGen.Generate(ctx)
			Expect(err).To(BeNil())
			Expect(key1).NotTo(Equal(key2))
		})
	})
})
