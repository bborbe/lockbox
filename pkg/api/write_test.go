// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

var _ = Describe("Write", func() {
	ctx := context.Background()

	Describe("CreateSecretRequest.Validate", func() {
		It("valid password body returns secret.Secret with password set", func() {
			req := api.CreateSecretRequest{
				ContentType: "password",
				Name:        "my-login",
				Username:    "alice",
				URL:         "https://example.com",
				Description: "my secret",
				SecretData: &api.SecretData{
					Password:   "s3cr3t",
					OtpKeyData: "JBSWY3DPEHPK3PXP", // accepted but not stored
				},
			}

			result, err := req.Validate(ctx)

			Expect(err).To(BeNil())
			Expect(result.ContentType).To(Equal("password"))
			Expect(result.Password).To(Equal("s3cr3t"))
			Expect(result.File).To(Equal(""))
			Expect(result.Name).To(Equal("my-login"))
			Expect(result.Username).To(Equal("alice"))
			Expect(result.URL).To(Equal("https://example.com"))
			Expect(result.Description).To(Equal("my secret"))
		})

		It("valid file body stores base64 string in File", func() {
			original := []byte("hello world")
			encoded := base64.StdEncoding.EncodeToString(original)

			req := api.CreateSecretRequest{
				ContentType: "file",
				Name:        "my-cert",
				SecretData: &api.SecretData{
					FileContent: encoded,
					Filename:    "cert.pem", // accepted but not stored
				},
			}

			result, err := req.Validate(ctx)

			Expect(err).To(BeNil())
			Expect(result.ContentType).To(Equal("file"))
			Expect(result.Password).To(Equal(""))
			Expect(result.File).To(Equal(encoded))
			Expect(result.Name).To(Equal("my-cert"))
		})

		It("returns 400 when content_type is absent", func() {
			req := api.CreateSecretRequest{
				SecretData: &api.SecretData{Password: "s3cr3t"},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for content_type cc", func() {
			req := api.CreateSecretRequest{
				ContentType: "cc",
				SecretData:  &api.SecretData{Password: "4111111111111111"},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for unsupported content_type bogus", func() {
			req := api.CreateSecretRequest{
				ContentType: "bogus",
				SecretData:  &api.SecretData{Password: "s3cr3t"},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 when secret_data is nil", func() {
			req := api.CreateSecretRequest{
				ContentType: "password",
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for password secret missing password", func() {
			req := api.CreateSecretRequest{
				ContentType: "password",
				SecretData:  &api.SecretData{Password: ""},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for file secret missing file_content", func() {
			req := api.CreateSecretRequest{
				ContentType: "file",
				SecretData:  &api.SecretData{FileContent: ""},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 for file secret with invalid base64", func() {
			req := api.CreateSecretRequest{
				ContentType: "file",
				SecretData:  &api.SecretData{FileContent: "!!!not base64!!!"},
			}

			result, err := req.Validate(ctx)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("CreateSecretRequest.ApplyUpdate", func() {
		existingPassword := secret.Secret{
			Name:        "old-name",
			Description: "old-desc",
			Username:    "old-user",
			URL:         "https://old.example.com",
			ContentType: "password",
			Password:    "old-password",
			File:        "",
		}

		It("overlays name and url, leaves password unchanged when secret_data is nil", func() {
			req := api.CreateSecretRequest{
				Name:       "new-name",
				URL:        "https://new.example.com",
				SecretData: nil,
			}

			result, err := req.ApplyUpdate(ctx, existingPassword)

			Expect(err).To(BeNil())
			Expect(result.Name).To(Equal("new-name"))
			Expect(result.URL).To(Equal("https://new.example.com"))
			Expect(result.Username).To(Equal("old-user"))
			Expect(result.Description).To(Equal("old-desc"))
			Expect(result.Password).To(Equal("old-password"))
			Expect(result.ContentType).To(Equal("password"))
		})

		It("replaces password when secret_data.password is provided", func() {
			req := api.CreateSecretRequest{
				SecretData: &api.SecretData{Password: "new-password"},
			}

			result, err := req.ApplyUpdate(ctx, existingPassword)

			Expect(err).To(BeNil())
			Expect(result.Password).To(Equal("new-password"))
			Expect(result.Name).To(Equal("old-name"))
			Expect(result.ContentType).To(Equal("password"))
		})

		It("ignores content_type in request body", func() {
			req := api.CreateSecretRequest{
				ContentType: "file", // should be ignored
				SecretData:  &api.SecretData{Password: "still-password"},
			}

			result, err := req.ApplyUpdate(ctx, existingPassword)

			Expect(err).To(BeNil())
			Expect(result.ContentType).To(Equal("password"))
			Expect(result.Password).To(Equal("still-password"))
		})

		It("returns 400 when secret_data is malformed for existing password type", func() {
			req := api.CreateSecretRequest{
				SecretData: &api.SecretData{
					Password: "",
				}, // empty password for existing password secret
			}

			result, err := req.ApplyUpdate(ctx, existingPassword)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		existingFile := secret.Secret{
			Name:        "old-file",
			ContentType: "file",
			File:        base64.StdEncoding.EncodeToString([]byte("old content")),
		}

		It("returns 400 when secret_data is malformed for existing file type", func() {
			req := api.CreateSecretRequest{
				SecretData: &api.SecretData{FileContent: "!!!bad base64!!!"},
			}

			result, err := req.ApplyUpdate(ctx, existingFile)

			Expect(result).To(Equal(secret.Secret{}))
			Expect(err).ToNot(BeNil())
			var coded libhttp.ErrorWithStatusCode
			Expect(errors.As(err, &coded)).To(BeTrue())
			Expect(coded.StatusCode()).To(Equal(http.StatusBadRequest))
		})

		It("replaces file value when valid base64 is provided for existing file secret", func() {
			encoded := base64.StdEncoding.EncodeToString([]byte("new content"))

			req := api.CreateSecretRequest{
				SecretData: &api.SecretData{FileContent: encoded},
			}

			result, err := req.ApplyUpdate(ctx, existingFile)

			Expect(err).To(BeNil())
			Expect(result.File).To(Equal(encoded))
			Expect(result.ContentType).To(Equal("file"))
		})
	})
})
