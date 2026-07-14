// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/lockbox/pkg/api"
)

//counterfeiter:generate -o ../../mocks/lockbox-client.go --fake-name LockboxClient . LockboxClient

// LockboxClient writes secrets into a running Lockbox instance via its HTTP
// API.
type LockboxClient interface {
	// Upsert creates or replaces the secret stored under hashid.
	Upsert(ctx context.Context, hashid string, req api.UpsertRequest) error
}

// NewLockboxClient returns a LockboxClient talking to baseURL, authenticating
// with HTTP Basic auth using username and password.
func NewLockboxClient(
	httpClient *http.Client,
	baseURL string,
	username string,
	password string,
) LockboxClient {
	return &lockboxClient{
		httpClient: httpClient,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		username:   username,
		password:   password,
	}
}

type lockboxClient struct {
	httpClient *http.Client
	baseURL    string
	username   string
	password   string
}

func (l *lockboxClient) Upsert(
	ctx context.Context,
	hashid string,
	upsertRequest api.UpsertRequest,
) error {
	// Marshaling the secret is intentional: this is the request body PUT to
	// Lockbox, which persists the migrated secret.
	body, err := json.Marshal(upsertRequest) //#nosec G117
	if err != nil {
		return errors.Wrap(ctx, err, "marshal upsert request failed")
	}

	url := l.baseURL + "/api/secrets/" + hashid + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(ctx, err, "build request failed")
	}
	req.SetBasicAuth(l.username, l.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(ctx, err, "put secret %s failed", hashid)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return errors.Wrapf(
			ctx,
			errUnexpectedStatus,
			"PUT %s returned status %d",
			url,
			resp.StatusCode,
		)
	}

	var result api.UpsertResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errors.Wrap(ctx, err, "decode upsert result failed")
	}
	return nil
}
