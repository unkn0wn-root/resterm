package ui

import (
	"context"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type explainAuthPreviewResult struct {
	status       xplain.StageStatus
	summary      string
	notes        []string
	extraSecrets []string
}

func explainAuthSecretValues(auth *restfile.AuthSpec, resolver *vars.Resolver) []string {
	return rqeng.AuthSecretValues(auth, resolver)
}

func explainInjectedAuthSecrets(
	auth *restfile.AuthSpec,
	before *restfile.Request,
	after *restfile.Request,
) []string {
	return rqeng.InjectedAuthSecrets(auth, before, after)
}

func commandAuthSecrets(res authcmd.Result) []string {
	return rqeng.CommandAuthSecrets(res)
}

func (m *Model) resolveInheritedAuth(doc *restfile.Document, req *restfile.Request) {
	m.requestSvc(httpclient.Options{}).ResolveInheritedAuth(doc, req)
}

func (m *Model) prepareExplainAuthPreview(
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (explainAuthPreviewResult, error) {
	preview, err := m.requestSvc(httpclient.Options{}).PrepareExplainAuthPreview(nil, req, resolver, envName)
	if err != nil {
		return explainAuthPreviewResult{}, err
	}
	return explainAuthPreviewResult{
		status:       preview.Status,
		summary:      preview.Summary,
		notes:        preview.Notes,
		extraSecrets: preview.ExtraSecrets,
	}, nil
}

func (m *Model) ensureCommandAuth(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
	timeout time.Duration,
) (authcmd.Result, error) {
	return m.requestSvc(httpclient.Options{}).EnsureCommandAuth(
		ctx,
		nil,
		req,
		resolver,
		envName,
		timeout,
	)
}

func (m *Model) prepareCommandAuth(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	envName string,
	timeout time.Duration,
) (authcmd.Prepared, error) {
	return m.requestSvc(httpclient.Options{}).PrepareCommandAuth(nil, auth, resolver, envName, timeout)
}

func (m *Model) ensureOAuth(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts httpclient.Options,
	envName string,
	timeout time.Duration,
) error {
	eng := m.requestSvc(opts)
	if req != nil &&
		req.Metadata.Auth != nil &&
		strings.EqualFold(req.Metadata.Auth.Type, "oauth2") &&
		timeout < 2*time.Minute {
		cfg, err := eng.ResolveOAuthConfig(req.Metadata.Auth, resolver, envName)
		if err != nil {
			return err
		}
		if (req.Headers == nil || req.Headers.Get(cfg.Header) == "") &&
			cfg.GrantType == oauth.GrantAuthorizationCode {
			m.setStatusMessage(
				statusMsg{
					level: statusInfo,
					text:  "Open browser to complete OAuth (auth code/PKCE). Press send again to cancel.",
				},
			)
		}
	}
	return eng.EnsureOAuth(ctx, req, resolver, opts, envName, timeout)
}

func (m *Model) buildCommandAuthConfig(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	timeout time.Duration,
) (authcmd.Config, error) {
	return m.requestSvc(httpclient.Options{}).BuildCommandAuthConfig(nil, auth, resolver, timeout)
}

func (m *Model) buildOAuthConfig(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
) (oauth.Config, error) {
	return m.requestSvc(httpclient.Options{}).BuildOAuthConfig(auth, resolver)
}
