// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/handler"
)

var _ = Describe("BasicAuth", func() {
	var httpHandler http.Handler
	var delegateCalled bool

	BeforeEach(func() {
		delegateCalled = false
		delegate := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			delegateCalled = true
			resp.WriteHeader(http.StatusOK)
		})
		httpHandler = handler.NewBasicAuth("user", "pass", delegate)
	})

	It("returns 401 when no credentials are supplied", func() {
		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		Expect(delegateCalled).To(BeFalse())
	})

	It("returns 401 when the username is wrong", func() {
		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		req.SetBasicAuth("wrong", "pass")
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		Expect(delegateCalled).To(BeFalse())
	})

	It("returns 401 when the password is wrong", func() {
		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		req.SetBasicAuth("user", "wrong")
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		Expect(delegateCalled).To(BeFalse())
	})

	It("passes through to the delegate when credentials are correct", func() {
		req := httptest.NewRequest(http.MethodGet, "/api/secrets/AbC123/", nil)
		req.SetBasicAuth("user", "pass")
		resp := httptest.NewRecorder()

		httpHandler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(delegateCalled).To(BeTrue())
	})
})
