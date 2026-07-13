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

var _ = Describe("RevisionDataHandler", func() {
	var (
		store       *mocks.SecretStore
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewRevisionDataHandler(store))
	})

	It("returns the revision data", func() {
		store.GetReturns(&secret.Secret{
			Username: "alice",
			Password: "s3cr3t",
			File:     "ZmlsZQ==",
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/secret-revisions/AbC123/data", nil)
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		var body api.RevisionData
		Expect(json.Unmarshal(resp.Body.Bytes(), &body)).To(BeNil())
		Expect(body.Password).To(Equal("s3cr3t"))
		Expect(body.File).To(Equal("ZmlsZQ=="))

		_, key := store.GetArgsForCall(0)
		Expect(key).To(Equal(secret.Key("AbC123")))
	})

	It("returns an error status when the store fails", func() {
		store.GetReturns(nil, errors.New("boom"))

		req := httptest.NewRequest(http.MethodGet, "/api/secret-revisions/AbC123/data", nil)
		req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).NotTo(Equal(http.StatusOK))
	})
})
