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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/mocks"
	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/handler"
	"github.com/bborbe/lockbox/pkg/secret"
)

var _ = Describe("SecretSearchHandler", func() {
	var (
		store       *mocks.SecretStore
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewSecretSearchHandler(store))
	})

	It("returns the matching keys as absolute api_url entries", func() {
		store.SearchReturns(secret.Keys{"AbC123", "DeF456"}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/secrets/?search=ab", nil)
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		var body api.SearchResults
		Expect(json.Unmarshal(resp.Body.Bytes(), &body)).To(BeNil())
		Expect(body.Results).To(HaveLen(2))
		Expect(body.Results[0].APIURL).To(Equal("http://example.com/api/secrets/AbC123/"))
		Expect(body.Results[1].APIURL).To(Equal("http://example.com/api/secrets/DeF456/"))

		_, query := store.SearchArgsForCall(0)
		Expect(query).To(Equal("ab"))
	})

	It("returns an error status when the store fails", func() {
		store.SearchReturns(nil, errors.New("boom"))

		req := httptest.NewRequest(http.MethodGet, "/api/secrets/", nil)
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).NotTo(Equal(http.StatusOK))
	})
})
