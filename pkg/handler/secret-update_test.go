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

var _ = Describe("SecretUpdateHandler", func() {
	var (
		store       *mocks.SecretStore
		httpHandler http.Handler
	)

	BeforeEach(func() {
		store = &mocks.SecretStore{}
		httpHandler = libhttp.NewJSONErrorHandler(handler.NewSecretUpdateHandler(store))
	})

	Context("200 metadata update", func() {
		It("updates name and url, keeps password and content_type unchanged", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Name:        "old",
				URL:         "https://old",
				Password:    "p",
			}, nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				Name: "new",
				URL:  "https://new",
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusOK))
			var repr api.SecretRepresentation
			Expect(json.Unmarshal(resp.Body.Bytes(), &repr)).To(BeNil())
			Expect(repr.URL).To(Equal("https://new"))
			Expect(repr.ContentType).To(Equal("password"))

			Expect(store.UpsertCallCount()).To(Equal(1))
			_, _, upserted := store.UpsertArgsForCall(0)
			Expect(upserted.Name).To(Equal("new"))
			Expect(upserted.URL).To(Equal("https://new"))
			Expect(upserted.Password).To(Equal("p"))
			Expect(upserted.ContentType).To(Equal("password"))
		})
	})

	Context("200 value replacement", func() {
		It("replaces password when secret_data is provided", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Name:        "name",
				Password:    "oldpw",
			}, nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				SecretData: &api.SecretData{
					Password: "newpw",
				},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusOK))

			Expect(store.UpsertCallCount()).To(Equal(1))
			_, _, upserted := store.UpsertArgsForCall(0)
			Expect(upserted.Password).To(Equal("newpw"))
			Expect(upserted.ContentType).To(Equal("password"))
		})
	})

	Context("content_type immutable", func() {
		It("ignores content_type in the body and keeps the existing type", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Name:        "name",
				Password:    "pw",
			}, nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				ContentType: "file",
				Name:        "newname",
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusOK))
			var repr api.SecretRepresentation
			Expect(json.Unmarshal(resp.Body.Bytes(), &repr)).To(BeNil())
			Expect(repr.ContentType).To(Equal("password"))

			Expect(store.UpsertCallCount()).To(Equal(1))
			_, _, upserted := store.UpsertArgsForCall(0)
			Expect(upserted.ContentType).To(Equal("password"))
			Expect(upserted.Name).To(Equal("newname"))
		})
	})

	Context("404", func() {
		It("returns 404 when the secret does not exist", func() {
			store.GetReturns(nil, errors.New("not found"))

			body, err := json.Marshal(api.CreateSecretRequest{Name: "new"})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusNotFound))
			Expect(store.UpsertCallCount()).To(Equal(0))
		})
	})

	Context("400 malformed secret_data", func() {
		It("returns 400 when secret_data.password is empty for a password secret", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Password:    "oldpw",
			}, nil)

			body, err := json.Marshal(api.CreateSecretRequest{
				SecretData: &api.SecretData{
					Password: "",
				},
			})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(store.UpsertCallCount()).To(Equal(0))
		})
	})

	Context("400 malformed JSON body", func() {
		It("returns 400 when the request body is not valid JSON", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Password:    "pw",
			}, nil)

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader([]byte("not-json")),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(store.UpsertCallCount()).To(Equal(0))
		})
	})

	Context("500 upsert failure", func() {
		It("returns 500 when the store fails to persist", func() {
			store.GetReturns(&secret.Secret{
				ContentType: "password",
				Password:    "pw",
			}, nil)
			store.UpsertReturns(errors.New("boom"))

			body, err := json.Marshal(api.CreateSecretRequest{Name: "newname"})
			Expect(err).To(BeNil())

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/secrets/AbC123/",
				bytes.NewReader(body),
			)
			req = mux.SetURLVars(req, map[string]string{"key": "AbC123"})
			resp := httptest.NewRecorder()

			httpHandler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusInternalServerError))
		})
	})
})
