// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
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

var _ = Describe("SecretMetadataHandler", func() {
	var (
		store       *mocks.SecretStore
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewSecretMetadataHandler(store))
	})

	It("returns the secret metadata with an absolute current_revision url", func() {
		store.GetReturns(&secret.Secret{
			Username: "alice",
			URL:      "https://example.com",
			Password: "s3cr3t",
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		var body api.SecretMetadata
		Expect(json.Unmarshal(resp.Body.Bytes(), &body)).To(BeNil())
		Expect(body.Username).To(Equal("alice"))
		Expect(body.URL).To(Equal("https://example.com"))
		Expect(
			body.CurrentRevision,
		).To(Equal("http://example.com/api/secret-revisions/AbC123/"))

		Expect(store.GetCallCount()).To(Equal(1))
		_, key := store.GetArgsForCall(0)
		Expect(key).To(Equal(secret.Key("AbC123")))
	})

	It("returns an error status when the store fails", func() {
		store.GetReturns(nil, errors.New("boom"))

		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).NotTo(Equal(http.StatusOK))
	})
})
