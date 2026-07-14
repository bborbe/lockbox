// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/lockbox/pkg/api"
)

//counterfeiter:generate -o ../../mocks/teamvault-client.go --fake-name TeamVaultClient . TeamVaultClient

// TeamVaultClient reads secrets from a running TeamVault instance via its
// HTTP API.
type TeamVaultClient interface {
	// ListSecrets returns every secret visible to the configured user,
	// following the DRF "next" pagination links until exhausted.
	ListSecrets(ctx context.Context) ([]TeamVaultSecret, error)
	// GetRevisionData fetches the decrypted payload of a secret revision.
	// revisionURL is TeamVaultSecret.CurrentRevision.
	GetRevisionData(ctx context.Context, revisionURL string) (api.RevisionData, error)
}

// NewTeamVaultClient returns a TeamVaultClient talking to baseURL, authenticating
// with HTTP Basic auth using username and password (an API token).
func NewTeamVaultClient(
	httpClient *http.Client,
	baseURL string,
	username string,
	password string,
) TeamVaultClient {
	return &teamVaultClient{
		httpClient: httpClient,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		username:   username,
		password:   password,
	}
}

type teamVaultClient struct {
	httpClient *http.Client
	baseURL    string
	username   string
	password   string
}

func (t *teamVaultClient) ListSecrets(ctx context.Context) ([]TeamVaultSecret, error) {
	var result []TeamVaultSecret
	url := t.baseURL + "/api/secrets/"
	for url != "" {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(ctx, err, "context cancelled while listing secrets")
		}

		page, err := t.getListPage(ctx, url)
		if err != nil {
			return nil, errors.Wrapf(ctx, err, "get secret list page %s failed", url)
		}
		result = append(result, page.Results...)

		if page.Next == nil {
			break
		}
		url = *page.Next
	}
	return result, nil
}

func (t *teamVaultClient) getListPage(
	ctx context.Context,
	url string,
) (*teamVaultListResponse, error) {
	resp, err := t.do(ctx, http.MethodGet, url)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "request failed")
	}
	defer resp.Body.Close()

	var page teamVaultListResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, errors.Wrap(ctx, err, "decode secret list failed")
	}
	return &page, nil
}

func (t *teamVaultClient) GetRevisionData(
	ctx context.Context,
	revisionURL string,
) (api.RevisionData, error) {
	dataURL := strings.TrimSuffix(revisionURL, "/") + "/data"

	resp, err := t.do(ctx, http.MethodGet, dataURL)
	if err != nil {
		return api.RevisionData{}, errors.Wrapf(
			ctx,
			err,
			"request revision data %s failed",
			dataURL,
		)
	}
	defer resp.Body.Close()

	var data api.RevisionData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return api.RevisionData{}, errors.Wrap(ctx, err, "decode revision data failed")
	}
	return data, nil
}

func (t *teamVaultClient) do(
	ctx context.Context,
	method string,
	url string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "build request failed")
	}
	req.SetBasicAuth(t.username, t.password)
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "do request failed")
	}
	if resp.StatusCode/100 != 2 {
		_ = resp.Body.Close()
		return nil, errors.Wrapf(
			ctx,
			errUnexpectedStatus,
			"%s %s returned status %d",
			method,
			url,
			resp.StatusCode,
		)
	}
	return resp, nil
}
