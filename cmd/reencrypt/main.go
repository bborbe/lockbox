// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command reencrypt is a one-shot re-encrypt sweep: it opens the secret store
// data directory, reads every stored secret through the configured keyring, and
// rewrites it sealed under the current primary key. After a full sweep no stored
// secret references a retired key, so old keys can be safely dropped from the
// configuration. The sweep is idempotent and crash-safe; re-running it leaves
// every secret readable and unchanged in value.
package main

import (
	"context"
	"os"

	libboltkv "github.com/bborbe/boltkv"
	"github.com/bborbe/errors"
	"github.com/bborbe/service"
	"github.com/golang/glog"

	"github.com/bborbe/lockbox/pkg/keyring"
	"github.com/bborbe/lockbox/pkg/secret"
)

func main() {
	os.Exit(service.MainCmd(context.Background(), &application{}))
}

type application struct {
	DataDir        string `required:"true"  arg:"datadir"         env:"DATADIR"                 usage:"data directory"`
	EncryptionKey  string `required:"false" arg:"encryption-key"  env:"LOCKBOX_ENCRYPTION_KEY"  usage:"base64-encoded AES key (16 or 32 raw bytes)"    display:"length"`
	EncryptionKeys string `required:"false" arg:"encryption-keys" env:"LOCKBOX_ENCRYPTION_KEYS" usage:"comma-separated base64 AES keys, primary first" display:"length"`
}

// Run performs the re-encrypt sweep: it builds the keyring from the configured
// environment, opens the data directory, and re-encrypts every stored secret under
// the current primary key.
func (a *application) Run(ctx context.Context) error {
	crypter, err := keyring.Parse(ctx, a.EncryptionKey, a.EncryptionKeys)
	if err != nil {
		return errors.Wrap(ctx, err, "build keyring failed")
	}

	db, err := libboltkv.OpenDir(ctx, a.DataDir)
	if err != nil {
		return errors.Wrap(ctx, err, "open db failed")
	}
	defer db.Close()

	store := secret.NewStore(db, crypter)

	glog.V(0).Infof("re-encrypting all secrets under the current primary key in %s", a.DataDir)
	if err := store.ReEncrypt(ctx); err != nil {
		return errors.Wrap(ctx, err, "re-encrypt sweep failed")
	}
	glog.V(0).Infof("re-encrypt sweep finished")
	return nil
}
