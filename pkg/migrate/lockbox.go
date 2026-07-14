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

// LockboxClient creates secrets in a running Lockbox instance via POST
// /api/secrets/ (TeamVault-compatible write API).
type LockboxClient interface {
	// Create sends a TeamVault create request to Lockbox's POST /api/secrets/
	// endpoint and returns the server-generated hashid on success.
	Create(ctx context.Context, req api.CreateSecretRequest) (string, error)
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

func (l *lockboxClient) Create(
	ctx context.Context,
	req api.CreateSecretRequest,
) (string, error) {
	// Marshaling the secret is intentional: this is the request body POST to
	// Lockbox, which persists the migrated secret.
	body, err := json.Marshal(req) //#nosec G117
	if err != nil {
		return "", errors.Wrap(ctx, err, "marshal create request failed")
	}

	url := l.baseURL + "/api/secrets/"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", errors.Wrap(ctx, err, "build request failed")
	}
	httpReq.SetBasicAuth(l.username, l.password)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return "", errors.Wrap(ctx, err, "post secret failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return "", errors.Wrapf(
			ctx,
			errUnexpectedStatus,
			"POST %s returned status %d",
			url,
			resp.StatusCode,
		)
	}

	var result api.SecretRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.Wrap(ctx, err, "decode create response failed")
	}
	return result.Hashid, nil
}
