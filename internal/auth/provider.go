// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/openapi"
)

type STSProvider struct {
	HTTPClient    *http.Client
	RefreshSkew   time.Duration
	TokenEndpoint string
}

func (p STSProvider) Credentials(ctx context.Context) (openapi.Credential, error) {
	tokenEndpoint := p.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = DefaultTokenEndpoint
	}
	token, _, err := EnsureFreshToken(ctx, p.HTTPClient, p.RefreshSkew, tokenEndpoint)
	if err != nil {
		return openapi.Credential{}, err
	}
	sts, err := ParseSTSCredential(token.AccessToken)
	if err != nil {
		return openapi.Credential{}, errs.New("vertc.auth.invalid_token", errs.TypeAuth,
			"stored access_token is not a valid STS credential: %s", err).
			WithHint("run `%s auth login` again", meta.BinName)
	}
	return openapi.Credential{
		AccessKeyID:     sts.AccessKeyID,
		SecretAccessKey: sts.SecretAccessKey,
		SessionToken:    sts.SessionToken,
	}, nil
}
