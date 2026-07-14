// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/mocks"
	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/handler"
	"github.com/bborbe/lockbox/pkg/secret"
)

var _ = Describe("SecretCreateHandler", func() {
	var (
		store       *mocks.SecretStore
		keyGen      *mocks.KeyGenerator
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		keyGen = &mocks.KeyGenerator{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewSecretCreateHandler(store, keyGen))
	})

	Context("201 happy path — password", func() {
		It("stores the secret and returns 201 with the representation", func() {
			keyGen.GenerateReturns(secret.Key("AbC12345"), nil)
			store.CreateReturns(nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				Name:        "my secret",
				Username:    "alice",
				URL:         "https://example.com/login",
				Description: "a test secret",
				SecretData: &api.SecretData{
					Password: "s3cr3t",
				},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusCreated))

			var repr api.SecretRepresentation
			Expect(json.Unmarshal(resp.Body.Bytes(), &repr)).To(BeNil())
			Expect(repr.Hashid).To(Equal("AbC12345"))
			Expect(repr.APIURL).To(Equal("http://example.com/api/secrets/AbC12345/"))
			Expect(repr.ContentType).To(Equal("password"))
			Expect(repr.Name).To(Equal("my secret"))
			Expect(repr.Username).To(Equal("alice"))
			Expect(repr.URL).To(Equal("https://example.com/login"))
			Expect(repr.Description).To(Equal("a test secret"))

			Expect(store.CreateCallCount()).To(Equal(1))
			_, key, value := store.CreateArgsForCall(0)
			Expect(key).To(Equal(secret.Key("AbC12345")))
			Expect(value.Password).To(Equal("s3cr3t"))
			Expect(value.Username).To(Equal("alice"))
			Expect(value.ContentType).To(Equal("password"))
		})
	})

	Context("201 happy path — file", func() {
		It("stores the file secret and returns 201", func() {
			keyGen.GenerateReturns(secret.Key("XyZ98765"), nil)
			store.CreateReturns(nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "file",
				Name:        "my file",
				SecretData: &api.SecretData{
					FileContent: "ZmlsZQ==", // "file" in base64
				},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusCreated))

			var repr api.SecretRepresentation
			Expect(json.Unmarshal(resp.Body.Bytes(), &repr)).To(BeNil())
			Expect(repr.ContentType).To(Equal("file"))
			Expect(repr.Hashid).To(Equal("XyZ98765"))
		})
	})

	DescribeTable("400 cases — validation failures",
		func(body api.CreateSecretRequest) {
			store.CreateReturns(nil)

			payload, err := json.Marshal(body)
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(payload))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(store.CreateCallCount()).To(Equal(0))
		},
		Entry("missing content_type", api.CreateSecretRequest{
			SecretData: &api.SecretData{Password: "s3cr3t"},
		}),
		Entry("unsupported content_type", api.CreateSecretRequest{
			ContentType: "cc",
			SecretData:  &api.SecretData{Password: "s3cr3t"},
		}),
		Entry("missing secret_data", api.CreateSecretRequest{
			ContentType: "password",
		}),
		Entry("password body missing password", api.CreateSecretRequest{
			ContentType: "password",
			SecretData:  &api.SecretData{},
		}),
		Entry("file body with invalid base64", api.CreateSecretRequest{
			ContentType: "file",
			SecretData:  &api.SecretData{FileContent: "not-valid-base64!!"},
		}),
	)

	Context("400 case — malformed JSON", func() {
		It("returns 400 without calling store.Create", func() {
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/secrets/",
				bytes.NewReader([]byte("not-json")),
			)
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(store.CreateCallCount()).To(Equal(0))
		})
	})

	Context("collision retry", func() {
		It("retries on ErrKeyExists and returns 201 on success", func() {
			keyGen.GenerateReturns(secret.Key("AbC12345"), nil)
			store.CreateStub = func(ctx context.Context, key secret.Key, s secret.Secret) error {
				if store.CreateCallCount() == 1 {
					return secret.ErrKeyExists
				}
				return nil
			}

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusCreated))
			Expect(store.CreateCallCount()).To(Equal(2))
			Expect(keyGen.GenerateCallCount()).To(Equal(2))
		})

		It("exhausts retries and returns 500 when store always returns ErrKeyExists", func() {
			keyGen.GenerateReturns(secret.Key("AbC12345"), nil)
			store.CreateReturns(secret.ErrKeyExists)

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusInternalServerError))
			Expect(store.CreateCallCount()).To(Equal(10))
		})
	})

	Context("backend error — 500", func() {
		It("returns 500 when store.Create returns a non-ErrKeyExists error", func() {
			keyGen.GenerateReturns(secret.Key("AbC12345"), nil)
			store.CreateReturns(errors.New("boom"))

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusInternalServerError))
			Expect(store.CreateCallCount()).To(Equal(1))
		})
	})

	Context("keygen error — 500", func() {
		It("returns 500 when keyGen.Generate fails", func() {
			keyGen.GenerateReturns("", errors.New("keygen broken"))

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusInternalServerError))
			Expect(store.CreateCallCount()).To(Equal(0))
		})
	})

	Context("context cancellation", func() {
		It("returns an error when context is cancelled during key generation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // cancel immediately

			keyGen.GenerateStub = func(ctx context.Context) (secret.Key, error) {
				return "", ctx.Err()
			}

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(http.MethodPost, "/api/secrets/", bytes.NewReader(body))
			req = req.WithContext(ctx)
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).NotTo(Equal(http.StatusCreated))
		})
	})
})
