// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"context"

	"github.com/bborbe/errors"
	"github.com/golang/glog"

	"github.com/bborbe/lockbox/pkg/api"
)

// Report summarizes the outcome of a Migrator run.
type Report struct {
	// Migrated is the number of secrets successfully written to Lockbox.
	Migrated int
	// SkippedUnreadable is the number of secrets skipped because the
	// authenticated TeamVault user could not read their revision data.
	SkippedUnreadable int
	// SkippedCC is the number of credit-card secrets skipped because
	// Lockbox has no field to hold them.
	SkippedCC int
	// Failed is the number of secrets where fetching from TeamVault or
	// writing to Lockbox errored.
	Failed int
}

//counterfeiter:generate -o ../../mocks/migrator.go --fake-name Migrator . Migrator

// Migrator copies every readable, non-credit-card secret from a TeamVault
// instance into a Lockbox instance.
type Migrator interface {
	// Run performs the migration and returns a Report of what happened. A
	// single secret failing to fetch or write is logged and counted in
	// Report.Failed; it never aborts the run. Run returns an error only when
	// listing secrets from TeamVault fails entirely or ctx is cancelled.
	Run(ctx context.Context) (Report, error)
}

// NewMigrator returns a Migrator that reads from source and writes to sink.
func NewMigrator(source TeamVaultClient, sink LockboxClient) Migrator {
	return &migrator{
		source: source,
		sink:   sink,
	}
}

type migrator struct {
	source TeamVaultClient
	sink   LockboxClient
}

func (m *migrator) Run(ctx context.Context) (Report, error) {
	var report Report

	secrets, err := m.source.ListSecrets(ctx)
	if err != nil {
		return report, errors.Wrap(ctx, err, "list teamvault secrets failed")
	}

	for _, secret := range secrets {
		if err := ctx.Err(); err != nil {
			return report, errors.Wrap(ctx, err, "context cancelled while migrating secrets")
		}

		switch {
		case !secret.DataReadable:
			glog.V(1).Infof("skip secret %s: data not readable", secret.Hashid)
			report.SkippedUnreadable++
		case secret.ContentType == ContentTypeCreditCard:
			glog.V(1).
				Infof("skip secret %s: credit-card secrets are not supported by lockbox", secret.Hashid)
			report.SkippedCC++
		default:
			if err := m.migrateOne(ctx, secret); err != nil {
				glog.Warningf("migrate secret %s failed: %v", secret.Hashid, err)
				report.Failed++
			} else {
				report.Migrated++
			}
		}
	}

	return report, nil
}

func (m *migrator) migrateOne(ctx context.Context, secret TeamVaultSecret) error {
	data, err := m.source.GetRevisionData(ctx, secret.CurrentRevision)
	if err != nil {
		return errors.Wrapf(ctx, err, "get revision data for %s failed", secret.Hashid)
	}

	if err := m.sink.Upsert(ctx, secret.Hashid, api.UpsertRequest{
		Username: secret.Username,
		URL:      secret.URL,
		Password: data.Password,
		File:     data.File,
	}); err != nil {
		return errors.Wrapf(ctx, err, "upsert secret %s into lockbox failed", secret.Hashid)
	}

	return nil
}
