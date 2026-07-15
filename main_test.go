// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bborbe/crypto"
	"github.com/bborbe/memorykv"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/keyring"
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

// doJSON performs an HTTP request with optional JSON body and optional Basic auth.
func doJSON(
	router *mux.Router,
	method, path string,
	auth bool,
	body any,
) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		data, err := json.Marshal(body)
		Expect(err).To(BeNil())
		req = httptest.NewRequest(method, path, &reader{data: data})
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if auth {
		req.SetBasicAuth("svc", "topsecret")
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// reader wraps a byte slice as a ReadCloser for http.Request body.
type reader struct{ data []byte }

func (r *reader) Read(p []byte) (n int, _ error) {
	n = copy(p, r.data)
	r.data = r.data[:0]
	return n, nil
}

func (r *reader) Close() error { return nil }

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

			// -------------------------------------------------------------------------
			// Auth — retained from prompt 5
			// -------------------------------------------------------------------------

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

			// -------------------------------------------------------------------------
			// POST create → read round-trip (password)
			// -------------------------------------------------------------------------

			Context("POST /secrets/ (password)", func() {
				It("creates a secret and the created secret is readable", func() {
					// POST create
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypePassword,
						Name:        "gh",
						Username:    "alice",
						URL:         "https://example.com",
						SecretData:  &api.SecretData{Password: "s3cr3t"},
					}
					postResp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp.Code).To(Equal(http.StatusCreated))

					var postResult api.SecretRepresentation
					err := json.NewDecoder(postResp.Body).Decode(&postResult)
					Expect(err).To(BeNil())
					Expect(postResult.Hashid).NotTo(BeEmpty())
					Expect(
						postResult.APIURL,
					).To(Equal("http://example.com" + prefix + "/secrets/" + postResult.Hashid + "/"))
					Expect(postResult.ContentType).To(Equal("password"))
					Expect(postResult.Name).To(Equal("gh"))
					Expect(postResult.Username).To(Equal("alice"))
					Expect(postResult.URL).To(Equal("https://example.com"))

					// GET metadata — body must be exactly {name, username, url, current_revision}
					metadataResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secrets/"+postResult.Hashid+"/",
						true,
						nil,
					)
					Expect(metadataResp.Code).To(Equal(http.StatusOK))

					var metadata map[string]any
					err = json.NewDecoder(metadataResp.Body).Decode(&metadata)
					Expect(err).To(BeNil())
					Expect(metadata).To(HaveKey("name"))
					Expect(metadata).To(HaveKey("username"))
					Expect(metadata).To(HaveKey("url"))
					Expect(metadata).To(HaveKey("current_revision"))
					Expect(metadata).To(HaveLen(4))
					Expect(metadata["name"]).To(Equal("gh"))
					Expect(metadata["username"]).To(Equal("alice"))
					Expect(metadata["url"]).To(Equal("https://example.com"))

					// GET revision-data — body must be exactly {password, file}
					revisionURLRaw, ok := metadata["current_revision"].(string)
					Expect(ok).To(BeTrue(), "current_revision should be a string")
					// TeamVault contract: current_revision ends at the revision base
					// ".../{key}/"; the client appends "data" to fetch the payload.
					// (Fetching current_revision directly would hide a "/data"-suffix bug.)
					Expect(
						revisionURLRaw,
					).To(HaveSuffix("/secret-revisions/" + postResult.Hashid + "/"))
					revisionResp := doJSON(router, http.MethodGet, revisionURLRaw+"data", true, nil)
					Expect(revisionResp.Code).To(Equal(http.StatusOK))

					var revisionData map[string]any
					err = json.NewDecoder(revisionResp.Body).Decode(&revisionData)
					Expect(err).To(BeNil())
					Expect(revisionData).To(HaveKey("password"))
					Expect(revisionData).To(HaveKey("file"))
					Expect(revisionData).To(HaveLen(2))
					Expect(revisionData["password"]).To(Equal("s3cr3t"))
					Expect(revisionData["file"]).To(Equal(""))
				})
			})

			// -------------------------------------------------------------------------
			// POST create (file)
			// -------------------------------------------------------------------------

			Context("POST /secrets/ (file)", func() {
				It("creates a file secret and the content is readable", func() {
					fileContent := base64.StdEncoding.EncodeToString([]byte("filecontent"))
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypeFile,
						Name:        "myfile",
						SecretData:  &api.SecretData{FileContent: fileContent, Filename: "f.txt"},
					}
					postResp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp.Code).To(Equal(http.StatusCreated))

					var postResult api.SecretRepresentation
					err := json.NewDecoder(postResp.Body).Decode(&postResult)
					Expect(err).To(BeNil())
					Expect(postResult.Hashid).NotTo(BeEmpty())

					// GET revision-data
					revisionResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secret-revisions/"+postResult.Hashid+"/data",
						true,
						nil,
					)
					Expect(revisionResp.Code).To(Equal(http.StatusOK))

					var revisionData map[string]any
					err = json.NewDecoder(revisionResp.Body).Decode(&revisionData)
					Expect(err).To(BeNil())
					Expect(revisionData["file"]).To(Equal(fileContent))
					Expect(revisionData["password"]).To(Equal(""))
				})
			})

			// -------------------------------------------------------------------------
			// POST 400 validation
			// -------------------------------------------------------------------------

			Context("POST /secrets/ validation errors", func() {
				DescribeTable("returns 400",
					func(body map[string]any, description string) {
						resp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, body)
						Expect(resp.Code).To(Equal(http.StatusBadRequest), description)
					},
					Entry("missing content_type", map[string]any{
						"secret_data": map[string]any{"password": "pw"},
					}, "content_type required"),
					Entry("unsupported content_type", map[string]any{
						"content_type": "cc",
						"secret_data":  map[string]any{"password": "pw"},
					}, "unsupported content_type"),
					Entry("missing secret_data", map[string]any{
						"content_type": "password",
					}, "secret_data required"),
					Entry("password body missing password", map[string]any{
						"content_type": "password",
						"secret_data":  map[string]any{},
					}, "password required for password secret"),
					Entry("file body missing file_content", map[string]any{
						"content_type": "file",
						"secret_data":  map[string]any{},
					}, "file_content required for file secret"),
					Entry("file body with invalid base64", map[string]any{
						"content_type": "file",
						"secret_data":  map[string]any{"file_content": "!!!"},
					}, "invalid base64"),
				)
			})

			// -------------------------------------------------------------------------
			// PATCH update metadata + value
			// -------------------------------------------------------------------------

			Context("PATCH /secrets/{hashid}/", func() {
				It("updates metadata and value", func() {
					// Create a password secret
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypePassword,
						Name:        "original",
						Username:    "bob",
						URL:         "https://old.example.com",
						SecretData:  &api.SecretData{Password: "originalPw"},
					}
					postResp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp.Code).To(Equal(http.StatusCreated))

					var postResult api.SecretRepresentation
					err := json.NewDecoder(postResp.Body).Decode(&postResult)
					Expect(err).To(BeNil())
					hashid := postResult.Hashid

					// PATCH metadata
					patchResp := doJSON(
						router,
						http.MethodPatch,
						prefix+"/secrets/"+hashid+"/",
						true,
						map[string]any{
							"name": "updated",
							"url":  "https://new.example.com",
						},
					)
					Expect(patchResp.Code).To(Equal(http.StatusOK))

					// Verify metadata changed
					metadataResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secrets/"+hashid+"/",
						true,
						nil,
					)
					Expect(metadataResp.Code).To(Equal(http.StatusOK))

					var metadata map[string]any
					err = json.NewDecoder(metadataResp.Body).Decode(&metadata)
					Expect(err).To(BeNil())
					Expect(metadata["url"]).To(Equal("https://new.example.com"))

					// PATCH secret_data (rotate password)
					patchDataResp := doJSON(
						router,
						http.MethodPatch,
						prefix+"/secrets/"+hashid+"/",
						true,
						map[string]any{
							"secret_data": map[string]any{"password": "rotated"},
						},
					)
					Expect(patchDataResp.Code).To(Equal(http.StatusOK))

					// Verify new password
					revisionResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secret-revisions/"+hashid+"/data",
						true,
						nil,
					)
					Expect(revisionResp.Code).To(Equal(http.StatusOK))

					var revisionData map[string]any
					err = json.NewDecoder(revisionResp.Body).Decode(&revisionData)
					Expect(err).To(BeNil())
					Expect(revisionData["password"]).To(Equal("rotated"))
				})
			})

			// -------------------------------------------------------------------------
			// content_type immutable on PATCH
			// -------------------------------------------------------------------------

			Context("PATCH /secrets/{hashid}/ content_type immutability", func() {
				It("ignores content_type in PATCH body and preserves the existing type", func() {
					// Create a password secret
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypePassword,
						SecretData:  &api.SecretData{Password: "pw"},
					}
					postResp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp.Code).To(Equal(http.StatusCreated))

					var postResult api.SecretRepresentation
					err := json.NewDecoder(postResp.Body).Decode(&postResult)
					Expect(err).To(BeNil())
					hashid := postResult.Hashid

					// PATCH with a different content_type — should be ignored
					patchResp := doJSON(
						router,
						http.MethodPatch,
						prefix+"/secrets/"+hashid+"/",
						true,
						map[string]any{
							"content_type": "file",
						},
					)
					Expect(patchResp.Code).To(Equal(http.StatusOK))

					var patchResult api.SecretRepresentation
					err = json.NewDecoder(patchResp.Body).Decode(&patchResult)
					Expect(err).To(BeNil())
					Expect(patchResult.ContentType).To(Equal("password"))

					// GET metadata still resolves
					metadataResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secrets/"+hashid+"/",
						true,
						nil,
					)
					Expect(metadataResp.Code).To(Equal(http.StatusOK))

					// Revision-data still returns password value
					revisionResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secret-revisions/"+hashid+"/data",
						true,
						nil,
					)
					Expect(revisionResp.Code).To(Equal(http.StatusOK))

					var revisionData map[string]any
					err = json.NewDecoder(revisionResp.Body).Decode(&revisionData)
					Expect(err).To(BeNil())
					Expect(revisionData["password"]).To(Equal("pw"))
				})
			})

			// -------------------------------------------------------------------------
			// Two identical POSTs yield distinct secrets
			// -------------------------------------------------------------------------

			Context("POST /secrets/ uniqueness", func() {
				It("creates two independent secrets with different hashids", func() {
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypePassword,
						SecretData:  &api.SecretData{Password: "samePw"},
					}

					postResp1 := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp1.Code).To(Equal(http.StatusCreated))
					var result1 api.SecretRepresentation
					Expect(json.NewDecoder(postResp1.Body).Decode(&result1)).To(Succeed())

					postResp2 := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp2.Code).To(Equal(http.StatusCreated))
					var result2 api.SecretRepresentation
					Expect(json.NewDecoder(postResp2.Body).Decode(&result2)).To(Succeed())

					Expect(result1.Hashid).NotTo(Equal(result2.Hashid))

					// Both are independently readable
					rev1 := doJSON(
						router,
						http.MethodGet,
						prefix+"/secret-revisions/"+result1.Hashid+"/data",
						true,
						nil,
					)
					rev2 := doJSON(
						router,
						http.MethodGet,
						prefix+"/secret-revisions/"+result2.Hashid+"/data",
						true,
						nil,
					)
					Expect(rev1.Code).To(Equal(http.StatusOK))
					Expect(rev2.Code).To(Equal(http.StatusOK))

					var data1, data2 map[string]any
					Expect(json.NewDecoder(rev1.Body).Decode(&data1)).To(Succeed())
					Expect(json.NewDecoder(rev2.Body).Decode(&data2)).To(Succeed())
					Expect(data1["password"]).To(Equal("samePw"))
					Expect(data2["password"]).To(Equal("samePw"))
				})
			})

			// -------------------------------------------------------------------------
			// PATCH on non-existent hashid → 404
			// -------------------------------------------------------------------------

			Context("PATCH /secrets/{hashid}/ 404", func() {
				It("returns 404 for a non-existent hashid", func() {
					resp := doJSON(
						router,
						http.MethodPatch,
						prefix+"/secrets/doesnotexist/",
						true,
						map[string]any{
							"name": "ignored",
						},
					)
					Expect(resp.Code).To(Equal(http.StatusNotFound))
				})
			})

			// -------------------------------------------------------------------------
			// 401 on write without/with wrong auth
			// -------------------------------------------------------------------------

			Context("write auth", func() {
				It("POST without auth returns 401", func() {
					resp := doJSON(
						router,
						http.MethodPost,
						prefix+"/secrets/",
						false,
						api.CreateSecretRequest{
							ContentType: secret.ContentTypePassword,
							SecretData:  &api.SecretData{Password: "pw"},
						},
					)
					Expect(resp.Code).To(Equal(http.StatusUnauthorized))
				})

				It("PATCH with wrong auth returns 401", func() {
					// Create a secret first so we have a valid hashid
					postResp := doJSON(
						router,
						http.MethodPost,
						prefix+"/secrets/",
						true,
						api.CreateSecretRequest{
							ContentType: secret.ContentTypePassword,
							SecretData:  &api.SecretData{Password: "pw"},
						},
					)
					Expect(postResp.Code).To(Equal(http.StatusCreated))
					var result api.SecretRepresentation
					Expect(json.NewDecoder(postResp.Body).Decode(&result)).To(Succeed())

					req := httptest.NewRequest(
						http.MethodPatch,
						prefix+"/secrets/"+result.Hashid+"/",
						&reader{data: []byte(`{}`)},
					)
					req.Header.Set("Content-Type", "application/json")
					req.SetBasicAuth(user, "wrong")
					resp := httptest.NewRecorder()
					router.ServeHTTP(resp, req)
					Expect(resp.Code).To(Equal(http.StatusUnauthorized))
				})
			})

			// -------------------------------------------------------------------------
			// Old flat PUT no longer creates a secret
			// -------------------------------------------------------------------------

			Context("PUT /secrets/{key}/ removed", func() {
				It("PUT does not create a secret and GET returns non-200", func() {
					req := httptest.NewRequest(http.MethodPut, prefix+"/secrets/somekey/", &reader{
						data: []byte(`{"content_type":"password","secret_data":{"password":"pw"}}`),
					})
					req.Header.Set("Content-Type", "application/json")
					req.SetBasicAuth(user, pass)
					resp := httptest.NewRecorder()
					router.ServeHTTP(resp, req)

					// PUT should not succeed (405 or 404 depending on router config)
					Expect(resp.Code).NotTo(Equal(http.StatusOK))
					Expect(resp.Code).NotTo(Equal(http.StatusCreated))

					// The key should not be readable
					getResp := doJSON(router, http.MethodGet, prefix+"/secrets/somekey/", true, nil)
					Expect(getResp.Code).NotTo(Equal(http.StatusOK))
				})
			})

			// -------------------------------------------------------------------------
			// GET /secrets/?search=q → TeamVault-compatible envelope
			// -------------------------------------------------------------------------

			Context("GET /secrets/?search=q", func() {
				It("returns a count/next/previous/results envelope with name metadata", func() {
					// Create a secret with a distinctive name
					postBody := api.CreateSecretRequest{
						ContentType: secret.ContentTypePassword,
						Name:        "searchable-name",
						Username:    "search-user",
						URL:         "https://search.example.com",
						SecretData:  &api.SecretData{Password: "search-pw"},
					}
					postResp := doJSON(router, http.MethodPost, prefix+"/secrets/", true, postBody)
					Expect(postResp.Code).To(Equal(http.StatusCreated))

					var postResult api.SecretRepresentation
					err := json.NewDecoder(postResp.Body).Decode(&postResult)
					Expect(err).To(BeNil())
					hashid := postResult.Hashid

					// Search for it
					searchResp := doJSON(
						router,
						http.MethodGet,
						prefix+"/secrets/?search=searchable",
						true,
						nil,
					)
					Expect(searchResp.Code).To(Equal(http.StatusOK))

					var searchBody map[string]any
					err = json.NewDecoder(searchResp.Body).Decode(&searchBody)
					Expect(err).To(BeNil())

					// Envelope shape
					Expect(searchBody).To(HaveKey("count"))
					Expect(searchBody).To(HaveKey("next"))
					Expect(searchBody).To(HaveKey("previous"))
					Expect(searchBody).To(HaveKey("results"))
					Expect(searchBody["count"]).To(BeNumerically(">=", 1))

					// Find the created secret's result by hashid and assert its fields
					results, ok := searchBody["results"].([]any)
					Expect(ok).To(BeTrue())
					var found bool
					for _, r := range results {
						result, ok := r.(map[string]any)
						Expect(ok).To(BeTrue())
						if result["hashid"] == hashid {
							found = true
							Expect(result["name"]).To(Equal("searchable-name"))
							Expect(result["username"]).To(Equal("search-user"))
							Expect(result["url"]).To(Equal("https://search.example.com"))
							Expect(result["api_url"]).To(Equal(
								"http://example.com" + prefix + "/secrets/" + hashid + "/",
							))
							// Secret values must NOT be present
							Expect(result).NotTo(HaveKey("password"))
							Expect(result).NotTo(HaveKey("file"))
							break
						}
					}
					Expect(found).To(BeTrue(), "search results should contain the created secret")
				})
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

	// -------------------------------------------------------------------------
	// Back-compat single-key still boots (AC 6)
	// -------------------------------------------------------------------------

	DescribeTable(
		"single-key legacy path",
		func(app *application) {
			crypter, err := app.createCrypter(ctx)
			Expect(err).To(BeNil())
			Expect(crypter).NotTo(BeNil())

			// End-to-end round-trip via secret.NewStore.
			db, err := memorykv.OpenMemory(ctx)
			Expect(err).To(BeNil())
			store := secret.NewStore(db, crypter)
			err = store.Upsert(ctx, secret.Key("testkey"), secret.Secret{Password: "pw"})
			Expect(err).To(BeNil())
			sec, err := store.Get(ctx, secret.Key("testkey"))
			Expect(err).To(BeNil())
			Expect(sec.Password).To(Equal("pw"))
		},
		Entry(
			"32-byte key",
			&application{EncryptionKey: base64.StdEncoding.EncodeToString(testEncryptionKey)},
		),
		Entry(
			"16-byte key",
			&application{
				EncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789012345")),
			},
		),
	)

	// -------------------------------------------------------------------------
	// Ordered multi-key config parses, primary first (AC 7)
	// -------------------------------------------------------------------------

	It("multi-key config: encrypt with primary, decrypt with primary-only ring", func() {
		key1 := crypto.SecretKey("11111111111111111111111111111111"[:32])
		key2 := crypto.SecretKey("22222222222222222222222222222222"[:32])

		app := &application{
			EncryptionKeys: base64.StdEncoding.EncodeToString(
				key1,
			) + "," + base64.StdEncoding.EncodeToString(
				key2,
			),
		}
		crypter, err := app.createCrypter(ctx)
		Expect(err).To(BeNil())
		Expect(crypter).NotTo(BeNil())

		// Encrypt a value.
		plaintext := []byte("topsecret")
		ciphertext, err := crypter.Encrypt(ctx, plaintext)
		Expect(err).To(BeNil())

		// Build a primary-only keyring and decrypt.
		ring, err := keyring.New(ctx, key1)
		Expect(err).To(BeNil())
		decrypted, err := ring.Decrypt(ctx, ciphertext)
		Expect(err).To(BeNil())
		Expect(decrypted).To(Equal(plaintext))
	})

	// -------------------------------------------------------------------------
	// Invalid or absent key material refuses start (AC 8)
	// -------------------------------------------------------------------------

	DescribeTable(
		"createCrypter refuses invalid config",
		func(app *application) {
			crypter, err := app.createCrypter(ctx)
			Expect(err).NotTo(BeNil())
			Expect(crypter).To(BeNil())
		},
		Entry("neither env var set", &application{}),
		Entry("both env vars set", &application{
			EncryptionKey:  base64.StdEncoding.EncodeToString(testEncryptionKey),
			EncryptionKeys: base64.StdEncoding.EncodeToString(testEncryptionKey),
		}),
		Entry("empty EncryptionKeys (comma-only)", &application{EncryptionKeys: ","}),
		Entry("empty EncryptionKeys (whitespace only)", &application{EncryptionKeys: "  ,  ,  "}),
		Entry("list entry not valid base64", &application{EncryptionKeys: "!!!"}),
		Entry(
			"list entry wrong length",
			&application{EncryptionKeys: base64.StdEncoding.EncodeToString([]byte("short"))},
		),
		Entry("duplicate keys in list", &application{
			EncryptionKeys: base64.StdEncoding.EncodeToString(
				testEncryptionKey,
			) + "," + base64.StdEncoding.EncodeToString(
				testEncryptionKey,
			),
		}),
	)
})
