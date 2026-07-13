// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"net/http"
	"os"
	"time"

	libboltkv "github.com/bborbe/boltkv"
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
	"github.com/bborbe/lockbox/pkg/secret"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN         string            `required:"true"  arg:"sentry-dsn"        env:"SENTRY_DSN"        usage:"SentryDSN"                                       display:"length"`
	SentryProxy       string            `required:"false" arg:"sentry-proxy"      env:"SENTRY_PROXY"      usage:"Sentry Proxy"`
	Listen            string            `required:"true"  arg:"listen"            env:"LISTEN"            usage:"address to listen to"`
	DataDir           string            `required:"true"  arg:"datadir"           env:"DATADIR"           usage:"data directory"`
	BasicAuthUsername string            `required:"true"  arg:"basic-auth-user"   env:"BASIC_AUTH_USER"   usage:"HTTP Basic auth username for the /api endpoints"`
	BasicAuthPassword string            `required:"true"  arg:"basic-auth-pass"   env:"BASIC_AUTH_PASS"   usage:"HTTP Basic auth password for the /api endpoints" display:"length"`
	BuildGitVersion   string            `required:"false" arg:"build-git-version" env:"BUILD_GIT_VERSION" usage:"Build Git version"                                                default:"dev"`
	BuildGitCommit    string            `required:"false" arg:"build-git-commit"  env:"BUILD_GIT_COMMIT"  usage:"Build Git commit hash"                                            default:"none"`
	BuildDate         *libtime.DateTime `required:"false" arg:"build-date"        env:"BUILD_DATE"        usage:"Build timestamp (RFC3339)"`
}

func (a *application) Run(ctx context.Context, sentryClient libsentry.Client) error {
	libmetrics.NewBuildInfoMetrics().SetBuildInfo(a.BuildGitVersion, a.BuildGitCommit, a.BuildDate)

	db, err := libboltkv.OpenDir(ctx, a.DataDir)
	if err != nil {
		return errors.Wrap(ctx, err, "open db failed")
	}
	defer db.Close()

	return service.Run(
		ctx,
		a.createHTTPServer(sentryClient, db),
	)
}

func (a *application) createHTTPServer(
	sentryClient libsentry.Client,
	db libkv.DB,
) run.Func {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		secretStore := secret.NewStore(db)

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
	router.Path(prefix + "/secrets/{key}/").Methods(http.MethodPut).
		Handler(auth(handler.NewSecretUpsertHandler(store)))
}
