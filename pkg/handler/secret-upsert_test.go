// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	libhttp "github.com/bborbe/http"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/mocks"
	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/handler"
	"github.com/bborbe/lockbox/pkg/secret"
)

var _ = Describe("SecretUpsertHandler", func() {
	var (
		store       *mocks.SecretStore
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewSecretUpsertHandler(store))
	})

	It("upserts the secret and returns its absolute metadata url", func() {
		payload := api.UpsertRequest{
			Username: "alice",
			URL:      "https://example.com",
			Password: "s3cr3t",
			File:     "ZmlsZQ==",
		}
		body, err := json.Marshal(payload)
		Expect(err).To(BeNil())

		req := httptest.NewRequest(http.MethodPut, "/api/secrets/AbC123/", bytes.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		var result api.UpsertResult
		Expect(json.Unmarshal(resp.Body.Bytes(), &result)).To(BeNil())
		Expect(result.APIURL).To(Equal("http://example.com/api/secrets/AbC123/"))

		Expect(store.UpsertCallCount()).To(Equal(1))
		_, key, value := store.UpsertArgsForCall(0)
		Expect(key).To(Equal(secret.Key("AbC123")))
		Expect(value).To(Equal(secret.Secret{
			Username: "alice",
			URL:      "https://example.com",
			Password: "s3cr3t",
			File:     "ZmlsZQ==",
		}))
	})

	It("returns an error status for an invalid body", func() {
		req := httptest.NewRequest(
			http.MethodPut,
			"/api/secrets/AbC123/",
			bytes.NewReader([]byte("not-json")),
		)
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).NotTo(Equal(http.StatusOK))
		Expect(store.UpsertCallCount()).To(Equal(0))
	})

	It("returns an error status when the store fails", func() {
		store.UpsertReturns(errors.New("boom"))

		body, err := json.Marshal(api.UpsertRequest{Username: "alice"})
		Expect(err).To(BeNil())

		req := httptest.NewRequest(http.MethodPut, "/api/secrets/AbC123/", bytes.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).NotTo(Equal(http.StatusOK))
	})
})
