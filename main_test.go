// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bborbe/memorykv"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

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
	store := secret.NewStore(db)

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

			It("round-trips PUT then GET metadata and revision data", func() {
				upsertBody, err := json.Marshal(api.UpsertRequest{
					Username: "alice",
					URL:      "https://example.com",
					Password: "s3cr3t",
					File:     "ZmlsZQ==",
				})
				Expect(err).To(BeNil())

				putReq := httptest.NewRequest(
					http.MethodPut,
					prefix+"/secrets/AbC123/",
					bytes.NewReader(upsertBody),
				)
				putReq.SetBasicAuth(user, pass)
				putResp := httptest.NewRecorder()
				router.ServeHTTP(putResp, putReq)
				Expect(putResp.Code).To(Equal(http.StatusOK))

				var upsertResult api.UpsertResult
				Expect(json.Unmarshal(putResp.Body.Bytes(), &upsertResult)).To(BeNil())
				Expect(
					upsertResult.APIURL,
				).To(Equal("http://example.com" + prefix + "/secrets/AbC123/"))

				metaReq := httptest.NewRequest(http.MethodGet, prefix+"/secrets/AbC123/", nil)
				metaReq.SetBasicAuth(user, pass)
				metaResp := httptest.NewRecorder()
				router.ServeHTTP(metaResp, metaReq)
				Expect(metaResp.Code).To(Equal(http.StatusOK))

				var meta map[string]any
				Expect(json.Unmarshal(metaResp.Body.Bytes(), &meta)).To(BeNil())
				Expect(meta).To(HaveKey("username"))
				Expect(meta).To(HaveKey("url"))
				Expect(meta).To(HaveKey("current_revision"))
				Expect(meta["username"]).To(Equal("alice"))
				Expect(meta["url"]).To(Equal("https://example.com"))
				Expect(
					meta["current_revision"],
				).To(Equal("http://example.com" + prefix + "/secret-revisions/AbC123/data"))

				dataReq := httptest.NewRequest(
					http.MethodGet,
					prefix+"/secret-revisions/AbC123/data",
					nil,
				)
				dataReq.SetBasicAuth(user, pass)
				dataResp := httptest.NewRecorder()
				router.ServeHTTP(dataResp, dataReq)
				Expect(dataResp.Code).To(Equal(http.StatusOK))

				var data map[string]any
				Expect(json.Unmarshal(dataResp.Body.Bytes(), &data)).To(BeNil())
				Expect(data).To(HaveKey("password"))
				Expect(data).To(HaveKey("file"))
				Expect(data["password"]).To(Equal("s3cr3t"))
				Expect(data["file"]).To(Equal("ZmlsZQ=="))

				searchReq := httptest.NewRequest(http.MethodGet, prefix+"/secrets/?search=", nil)
				searchReq.SetBasicAuth(user, pass)
				searchResp := httptest.NewRecorder()
				router.ServeHTTP(searchResp, searchReq)
				Expect(searchResp.Code).To(Equal(http.StatusOK))

				var results map[string]any
				Expect(json.Unmarshal(searchResp.Body.Bytes(), &results)).To(BeNil())
				Expect(results).To(HaveKey("results"))
				list, ok := results["results"].([]any)
				Expect(ok).To(BeTrue())
				Expect(list).To(HaveLen(1))
				entry, ok := list[0].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(entry).To(HaveKey("api_url"))
				Expect(
					entry["api_url"],
				).To(Equal("http://example.com" + prefix + "/secrets/AbC123/"))
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
