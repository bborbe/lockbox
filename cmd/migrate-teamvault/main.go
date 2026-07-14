// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command migrate-teamvault is a one-shot API-to-API importer: it reads all
// secrets from a running TeamVault instance via its HTTP API and creates them
// via POST /api/secrets/ in a running Lockbox instance. A single secret
// failing to read or write is logged and skipped; the run continues.
package main

import (
	"context"
	"os"
	"time"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	"github.com/bborbe/service"
	"github.com/golang/glog"

	"github.com/bborbe/lockbox/pkg/migrate"
)

func main() {
	os.Exit(service.MainCmd(context.Background(), &application{}))
}

type application struct {
	TeamVaultURL  string `required:"true" arg:"teamvault-url"  env:"TEAMVAULT_URL"  usage:"TeamVault base URL, e.g. https://teamvault.example.com"`
	TeamVaultUser string `required:"true" arg:"teamvault-user" env:"TEAMVAULT_USER" usage:"TeamVault HTTP Basic auth username"`
	TeamVaultPass string `required:"true" arg:"teamvault-pass" env:"TEAMVAULT_PASS" usage:"TeamVault HTTP Basic auth password (API token)"         display:"length"`
	LockboxURL    string `required:"true" arg:"lockbox-url"    env:"LOCKBOX_URL"    usage:"Lockbox base URL, e.g. http://localhost:8080"`
	LockboxUser   string `required:"true" arg:"lockbox-user"   env:"LOCKBOX_USER"   usage:"Lockbox HTTP Basic auth username"`
	LockboxPass   string `required:"true" arg:"lockbox-pass"   env:"LOCKBOX_PASS"   usage:"Lockbox HTTP Basic auth password"                       display:"length"`
}

// Run performs the migration: it reads every secret from TeamVault and creates
// the readable, non-credit-card ones in Lockbox via POST /api/secrets/, then
// logs a summary.
func (a *application) Run(ctx context.Context) error {
	httpClient := libhttp.CreateHTTPClient(30 * time.Second)

	teamVaultClient := migrate.NewTeamVaultClient(
		httpClient,
		a.TeamVaultURL,
		a.TeamVaultUser,
		a.TeamVaultPass,
	)
	lockboxClient := migrate.NewLockboxClient(
		httpClient,
		a.LockboxURL,
		a.LockboxUser,
		a.LockboxPass,
	)
	migrator := migrate.NewMigrator(teamVaultClient, lockboxClient)

	glog.V(0).Infof("migrating secrets from %s to %s", a.TeamVaultURL, a.LockboxURL)
	report, err := migrator.Run(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "migration failed")
	}

	glog.V(0).Infof(
		"migration finished: migrated=%d skipped_unreadable=%d skipped_cc=%d failed=%d",
		report.Migrated,
		report.SkippedUnreadable,
		report.SkippedCC,
		report.Failed,
	)
	return nil
}
