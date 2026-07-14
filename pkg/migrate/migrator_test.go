// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/migrate"
)

var _ = Describe("Migrator", func() {
	var (
		ctx            context.Context
		teamVaultCalls []string
		lockboxServer  *httptest.Server
		teamVaultURL   string
		lockboxMu      sync.Mutex
		lockboxPuts    map[string]api.UpsertRequest
		report         migrate.Report
		runErr         error
	)

	BeforeEach(func() {
		ctx = context.Background()
		teamVaultCalls = nil
		lockboxPuts = map[string]api.UpsertRequest{}

		teamVaultMux := http.NewServeMux()
		teamVaultServer := httptest.NewServer(teamVaultMux)
		DeferCleanup(teamVaultServer.Close)
		teamVaultURL = teamVaultServer.URL

		teamVaultMux.HandleFunc("/api/secrets/", func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			Expect(ok).To(BeTrue())
			Expect(username).To(Equal("tv-user"))
			Expect(password).To(Equal("tv-pass"))

			teamVaultCalls = append(teamVaultCalls, r.URL.String())

			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"count":    6,
					"next":     nil,
					"previous": teamVaultURL + "/api/secrets/",
					"results": []map[string]any{
						{
							"hashid":           "ffff6666",
							"username":         "page2user",
							"url":              "https://page2.example.com",
							"content_type":     "password",
							"current_revision": teamVaultURL + "/api/secret-revisions/rev-page2/",
							"data_readable":    true,
							"name":             "page2 secret",
							"status":           "ok",
						},
					},
				})
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"count":    6,
				"next":     teamVaultURL + "/api/secrets/?page=2",
				"previous": nil,
				"results": []map[string]any{
					{
						"hashid":           "aaaa1111",
						"username":         "pwuser",
						"url":              "https://password.example.com",
						"content_type":     "password",
						"current_revision": teamVaultURL + "/api/secret-revisions/rev-pw/",
						"data_readable":    true,
						"name":             "password secret",
						"status":           "ok",
					},
					{
						"hashid":           "bbbb2222",
						"username":         "fileuser",
						"url":              "https://file.example.com",
						"content_type":     "file",
						"current_revision": teamVaultURL + "/api/secret-revisions/rev-file/",
						"data_readable":    true,
						"name":             "file secret",
						"status":           "ok",
					},
					{
						"hashid":           "cccc3333",
						"username":         "ccuser",
						"url":              "",
						"content_type":     "cc",
						"current_revision": teamVaultURL + "/api/secret-revisions/rev-cc/",
						"data_readable":    true,
						"name":             "credit card secret",
						"status":           "ok",
					},
					{
						"hashid":           "dddd4444",
						"username":         "unreadableuser",
						"url":              "",
						"content_type":     "password",
						"current_revision": teamVaultURL + "/api/secret-revisions/rev-unreadable/",
						"data_readable":    false,
						"name":             "unreadable secret",
						"status":           "ok",
					},
					{
						"hashid":           "eeee5555",
						"username":         "erroruser",
						"url":              "",
						"content_type":     "password",
						"current_revision": teamVaultURL + "/api/secret-revisions/rev-error/",
						"data_readable":    true,
						"name":             "error secret",
						"status":           "ok",
					},
				},
			})
		})

		teamVaultMux.HandleFunc(
			"/api/secret-revisions/rev-pw/data",
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"password": "s3cr3t"})
			},
		)
		teamVaultMux.HandleFunc(
			"/api/secret-revisions/rev-file/data",
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"file": "ZmlsZWNvbnRlbnQ="})
			},
		)
		teamVaultMux.HandleFunc(
			"/api/secret-revisions/rev-page2/data",
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"password": "page2pw"})
			},
		)
		teamVaultMux.HandleFunc(
			"/api/secret-revisions/rev-error/data",
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		)
		// rev-cc and rev-unreadable are intentionally not registered: the
		// migrator must never call them (cc / unreadable are skipped before
		// fetching revision data).

		lockboxMux := http.NewServeMux()
		lockboxServer = httptest.NewServer(lockboxMux)
		DeferCleanup(lockboxServer.Close)

		lockboxMux.HandleFunc("/api/secrets/", func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPut))

			username, password, ok := r.BasicAuth()
			Expect(ok).To(BeTrue())
			Expect(username).To(Equal("lb-user"))
			Expect(password).To(Equal("lb-pass"))

			hashid := r.URL.Path[len("/api/secrets/") : len(r.URL.Path)-1]

			var body api.UpsertRequest
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())

			lockboxMu.Lock()
			lockboxPuts[hashid] = body
			lockboxMu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.UpsertResult{
				APIURL: lockboxServer.URL + fmt.Sprintf("/api/secrets/%s/", hashid),
			})
		})
	})

	JustBeforeEach(func() {
		teamVaultClient := migrate.NewTeamVaultClient(
			http.DefaultClient,
			teamVaultURL,
			"tv-user",
			"tv-pass",
		)
		lockboxClient := migrate.NewLockboxClient(
			http.DefaultClient,
			lockboxServer.URL,
			"lb-user",
			"lb-pass",
		)
		migrator := migrate.NewMigrator(teamVaultClient, lockboxClient)
		report, runErr = migrator.Run(ctx)
	})

	It("does not return an error", func() {
		Expect(runErr).To(BeNil())
	})

	It("follows pagination to fetch every page", func() {
		Expect(teamVaultCalls).To(HaveLen(2))
	})

	It("counts migrated, skipped and failed secrets correctly", func() {
		Expect(report.Migrated).To(Equal(3))
		Expect(report.SkippedCC).To(Equal(1))
		Expect(report.SkippedUnreadable).To(Equal(1))
		Expect(report.Failed).To(Equal(1))
	})

	It("maps a password secret correctly", func() {
		lockboxMu.Lock()
		defer lockboxMu.Unlock()
		Expect(lockboxPuts).To(HaveKey("aaaa1111"))
		Expect(lockboxPuts["aaaa1111"]).To(Equal(api.UpsertRequest{
			Username: "pwuser",
			URL:      "https://password.example.com",
			Password: "s3cr3t",
			File:     "",
		}))
	})

	It("maps a file secret correctly", func() {
		lockboxMu.Lock()
		defer lockboxMu.Unlock()
		Expect(lockboxPuts).To(HaveKey("bbbb2222"))
		Expect(lockboxPuts["bbbb2222"]).To(Equal(api.UpsertRequest{
			Username: "fileuser",
			URL:      "https://file.example.com",
			Password: "",
			File:     "ZmlsZWNvbnRlbnQ=",
		}))
	})

	It("migrates the secret found on the second page", func() {
		lockboxMu.Lock()
		defer lockboxMu.Unlock()
		Expect(lockboxPuts).To(HaveKey("ffff6666"))
		Expect(lockboxPuts["ffff6666"].Password).To(Equal("page2pw"))
	})

	It("never writes credit-card, unreadable or failed secrets to lockbox", func() {
		lockboxMu.Lock()
		defer lockboxMu.Unlock()
		Expect(lockboxPuts).NotTo(HaveKey("cccc3333"))
		Expect(lockboxPuts).NotTo(HaveKey("dddd4444"))
		Expect(lockboxPuts).NotTo(HaveKey("eeee5555"))
	})
})
