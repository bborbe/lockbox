// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bborbe/crypto"
	"github.com/bborbe/memorykv"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/bborbe/lockbox/pkg/secret"
)

// testEncryptionKey is a fixed 32-byte AES-256 key used only in tests.
var testEncryptionKey = crypto.SecretKey("01234567890123456789012345678901"[:32])

func TestContractSuite(t *testing.T) {
	format.TruncatedDiff = false
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	RunSpecs(t, "Main Contract Suite", suiteConfig, reporterConfig)
}

// newContractRouter mirrors main.go's route wiring — dual /api and /api/v1
// prefixes, Basic-auth-protected — on an in-memory kv backend, for
// TeamVault-contract testing without a real database or process.
func newContractRouter(username, password string) *mux.Router {
	db, err := memorykv.OpenMemory(context.Background())
	Expect(err).To(BeNil())
	store := secret.NewStore(db, crypto.NewCrypter(testEncryptionKey))

	app := &application{
		BasicAuthUsername: username,
		BasicAuthPassword: password,
	}
	router := mux.NewRouter()
	app.registerAPI(router, "/api", store)
	app.registerAPI(router, "/api/v1", store)
	return router
}

var _ = Describe("TeamVault contract", func() {
	const user = "svc"
	const pass = "topsecret"

	for _, prefix := range []string{"/api", "/api/v1"} {
		prefix := prefix

		Describe("prefix "+prefix, func() {
			var router *mux.Router

			BeforeEach(func() {
				router = newContractRouter(user, pass)
			})

			It("rejects wrong Basic auth with 401", func() {
				req := httptest.NewRequest(http.MethodGet, prefix+"/secrets/AbC123/", nil)
				req.SetBasicAuth(user, "wrong")
				resp := httptest.NewRecorder()

				router.ServeHTTP(resp, req)

				Expect(resp.Code).To(Equal(http.StatusUnauthorized))
			})

			It("rejects missing Basic auth with 401", func() {
				req := httptest.NewRequest(http.MethodGet, prefix+"/secrets/AbC123/", nil)
				resp := httptest.NewRecorder()

				router.ServeHTTP(resp, req)

				Expect(resp.Code).To(Equal(http.StatusUnauthorized))
			})
		})
	}
})

var _ = Describe("createCrypter", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("accepts a base64-encoded 32-byte key", func() {
		app := &application{EncryptionKey: base64.StdEncoding.EncodeToString(testEncryptionKey)}
		crypter, err := app.createCrypter(ctx)
		Expect(err).To(BeNil())
		Expect(crypter).NotTo(BeNil())
	})

	It("accepts a base64-encoded 16-byte key", func() {
		app := &application{
			EncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789012345")),
		}
		crypter, err := app.createCrypter(ctx)
		Expect(err).To(BeNil())
		Expect(crypter).NotTo(BeNil())
	})

	It("rejects a key of invalid length", func() {
		app := &application{
			EncryptionKey: base64.StdEncoding.EncodeToString([]byte("too-short")),
		}
		_, err := app.createCrypter(ctx)
		Expect(err).NotTo(BeNil())
	})

	It("rejects a non-base64 key", func() {
		app := &application{EncryptionKey: "not-valid-base64!!"}
		_, err := app.createCrypter(ctx)
		Expect(err).NotTo(BeNil())
	})
})
