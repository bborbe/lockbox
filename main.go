// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	libboltkv "github.com/bborbe/boltkv"
	"github.com/bborbe/crypto"
	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	libkv "github.com/bborbe/kv"
	"github.com/bborbe/log"
	libmetrics "github.com/bborbe/metrics"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bborbe/lockbox/pkg/factory"
	"github.com/bborbe/lockbox/pkg/handler"
	"github.com/bborbe/lockbox/pkg/keyring"
	"github.com/bborbe/lockbox/pkg/secret"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN         string            `required:"false" arg:"sentry-dsn"        env:"SENTRY_DSN"              usage:"Sentry DSN (optional; empty disables Sentry)"                                                                             display:"length"`
	SentryProxy       string            `required:"false" arg:"sentry-proxy"      env:"SENTRY_PROXY"            usage:"Sentry Proxy"`
	Listen            string            `required:"true"  arg:"listen"            env:"LISTEN"                  usage:"address to listen to"`
	DataDir           string            `required:"true"  arg:"datadir"           env:"DATADIR"                 usage:"data directory"`
	BasicAuthUsername string            `required:"true"  arg:"basic-auth-user"   env:"BASIC_AUTH_USER"         usage:"HTTP Basic auth username for the /api endpoints"`
	BasicAuthPassword string            `required:"true"  arg:"basic-auth-pass"   env:"BASIC_AUTH_PASS"         usage:"HTTP Basic auth password for the /api endpoints"                                                                          display:"length"`
	EncryptionKey     string            `required:"false" arg:"encryption-key"    env:"LOCKBOX_ENCRYPTION_KEY"  usage:"base64-encoded AES key (16 or 32 raw bytes) used to encrypt stored secrets"                                               display:"length"`
	EncryptionKeys    string            `required:"false" arg:"encryption-keys"   env:"LOCKBOX_ENCRYPTION_KEYS" usage:"comma-separated base64 AES keys (16 or 32 raw bytes each), primary first; mutually exclusive with LOCKBOX_ENCRYPTION_KEY" display:"length"`
	BuildGitVersion   string            `required:"false" arg:"build-git-version" env:"BUILD_GIT_VERSION"       usage:"Build Git version"                                                                                                                         default:"dev"`
	BuildGitCommit    string            `required:"false" arg:"build-git-commit"  env:"BUILD_GIT_COMMIT"        usage:"Build Git commit hash"                                                                                                                     default:"none"`
	BuildDate         *libtime.DateTime `required:"false" arg:"build-date"        env:"BUILD_DATE"              usage:"Build timestamp (RFC3339)"`
}

func (a *application) Run(ctx context.Context, sentryClient libsentry.Client) error {
	libmetrics.NewBuildInfoMetrics().SetBuildInfo(a.BuildGitVersion, a.BuildGitCommit, a.BuildDate)

	crypter, err := a.createCrypter(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "create crypter failed")
	}

	db, err := libboltkv.OpenDir(ctx, a.DataDir)
	if err != nil {
		return errors.Wrap(ctx, err, "open db failed")
	}
	defer db.Close()

	return service.Run(
		ctx,
		a.createHTTPServer(sentryClient, db, crypter),
	)
}

// createCrypter returns a Crypter backed by a keyring. Exactly one of
// LOCKBOX_ENCRYPTION_KEY (single key) or LOCKBOX_ENCRYPTION_KEYS (comma-separated
// base64 list, primary first) must be set; the server refuses to start if neither
// is set, both are set, or any key material is invalid (bad base64, wrong byte
// length, or duplicate keys).
func (a *application) createCrypter(ctx context.Context) (crypto.Crypter, error) {
	single := strings.TrimSpace(a.EncryptionKey)
	list := strings.TrimSpace(a.EncryptionKeys)

	// Exactly-one rule.
	if single == "" && list == "" {
		return nil, errors.New(
			ctx,
			"either LOCKBOX_ENCRYPTION_KEY or LOCKBOX_ENCRYPTION_KEYS must be set",
		)
	}
	if single != "" && list != "" {
		return nil, errors.New(
			ctx,
			"LOCKBOX_ENCRYPTION_KEY and LOCKBOX_ENCRYPTION_KEYS are mutually exclusive; set exactly one",
		)
	}

	// Collect base64 entries.
	var entries []string
	if list != "" {
		parts := strings.Split(list, ",")
		for _, p := range parts {
			entries = append(entries, strings.TrimSpace(p))
		}
	} else {
		entries = []string{single}
	}

	// Reject empty/whitespace entries.
	for i, e := range entries {
		if e == "" {
			return nil, errors.Errorf(ctx, "LOCKBOX_ENCRYPTION_KEYS entry %d is empty", i)
		}
	}

	// Decode and validate each key.
	keys := make([]crypto.SecretKey, 0, len(entries))
	for i, entry := range entries {
		raw, err := base64.StdEncoding.DecodeString(entry)
		if err != nil {
			return nil, errors.Wrapf(
				ctx,
				err,
				"LOCKBOX_ENCRYPTION_KEYS entry %d: base64 decode failed",
				i,
			)
		}
		if len(raw) != 16 && len(raw) != 32 {
			return nil, errors.Errorf(
				ctx,
				"LOCKBOX_ENCRYPTION_KEYS entry %d: must decode to 16 or 32 bytes, got %d",
				i,
				len(raw),
			)
		}
		keys = append(keys, crypto.SecretKey(raw))
	}

	// Build keyring (it rejects duplicates and empty input).
	ring, err := keyring.New(ctx, keys...)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "build keyring failed")
	}
	return ring, nil
}

func (a *application) createHTTPServer(
	sentryClient libsentry.Client,
	db libkv.DB,
	crypter crypto.Crypter,
) run.Func {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		secretStore := secret.NewStore(db, crypter)

		router := mux.NewRouter()

		// Admin endpoints — no auth (gateway-only surface).
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		router.Path("/resetdb").Handler(libkv.NewResetHandler(db, cancel))
		router.Path("/resetbucket/{BucketName}").Handler(libkv.NewResetBucketHandler(db, cancel))
		router.Path("/setloglevel/{level}").
			Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
		router.Path("/gc").Handler(libhttp.NewGarbageCollectorHandler())
		router.Path("/testloglevel").Handler(factory.CreateTestLoglevelHandler())
		router.Path("/sentryalert").Handler(factory.CreateSentryAlertHandler(sentryClient))

		// Business API — TeamVault-compatible, Basic-auth protected, on both the
		// unversioned and the /api/v1 prefix.
		a.registerAPI(router, "/api", secretStore)
		a.registerAPI(router, "/api/v1", secretStore)

		glog.V(2).Infof("starting http server listen on %s", a.Listen)
		return libhttp.NewServer(
			a.Listen,
			router,
		).Run(ctx)
	}
}

// registerAPI mounts the TeamVault-compatible read endpoints under prefix,
// each wrapped in Basic auth and JSON error handling.
func (a *application) registerAPI(router *mux.Router, prefix string, store secret.Store) {
	auth := func(h libhttp.WithError) http.Handler {
		return handler.NewBasicAuth(
			a.BasicAuthUsername,
			a.BasicAuthPassword,
			libhttp.NewJSONErrorHandler(h),
		)
	}
	router.Path(prefix + "/secrets/{key}/").Methods(http.MethodGet).
		Handler(auth(handler.NewSecretMetadataHandler(store)))
	router.Path(prefix + "/secret-revisions/{key}/data").Methods(http.MethodGet).
		Handler(auth(handler.NewRevisionDataHandler(store)))
	router.Path(prefix + "/secrets/").Methods(http.MethodGet).
		Handler(auth(handler.NewSecretSearchHandler(store)))
	router.Path(prefix + "/secrets/{key}/").Methods(http.MethodPatch).
		Handler(auth(handler.NewSecretUpdateHandler(store)))
	router.Path(prefix + "/secrets/").Methods(http.MethodPost).
		Handler(auth(handler.NewSecretCreateHandler(store, secret.NewKeyGenerator(secret.DefaultKeyLength))))
}
