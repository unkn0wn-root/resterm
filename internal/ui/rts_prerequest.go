package ui

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) runRTSPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	variables map[string]string,
	globals map[string]vars.GlobalMutation,
) (prerequest.Output, error) {
	return m.requestSvc(httpclient.Options{}).
		RunPreRequest(ctx, doc, req, envName, base, variables, globals)
}
