package ui

import (
	"maps"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) rtsPos(doc *restfile.Document, req *restfile.Request) vars.ExprPos {
	path := m.documentRuntimePath(doc)
	line := 1
	if req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	if strings.TrimSpace(path) == "" {
		path = m.currentFile
	}
	return vars.ExprPos{Path: path, Line: line, Col: 1}
}

func (m *Model) rtsPosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	path := m.documentRuntimePath(doc)
	if strings.TrimSpace(path) == "" {
		path = m.currentFile
	}
	if line <= 0 && req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	if line <= 0 {
		line = 1
	}
	return rts.Pos{Path: path, Line: line, Col: 1}
}

func (m *Model) rtsBase(doc *restfile.Document, base string) string {
	if strings.TrimSpace(base) != "" {
		return base
	}
	path := m.documentRuntimePath(doc)
	if strings.TrimSpace(path) != "" {
		return filepath.Dir(path)
	}
	if strings.TrimSpace(m.currentFile) != "" {
		return filepath.Dir(m.currentFile)
	}
	return ""
}

func (m *Model) rtsVars(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	extras ...map[string]string,
) map[string]string {
	res := m.collectVariables(doc, req, envName)
	for _, extra := range extras {
		maps.Copy(res, extra)
	}
	return res
}

func (m *Model) rtsVarsSafe(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	extras ...map[string]string,
) map[string]string {
	res := make(map[string]string)
	if env := vars.EnvValues(m.cfg.EnvironmentSet, envName); len(env) > 0 {
		maps.Copy(res, env)
	}

	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
	}

	m.mergeFileRuntimeVarsSafe(res, doc, envName)

	if gs := m.globalsStore(); gs != nil {
		if snap := gs.Snapshot(envName); len(snap) > 0 {
			for k, e := range snap {
				if e.Secret {
					continue
				}
				name := strings.TrimSpace(e.Name)
				if name == "" {
					name = k
				}
				res[name] = e.Value
			}
		}
	}

	if req != nil {
		for _, v := range req.Variables {
			if v.Secret {
				continue
			}
			res[v.Name] = v.Value
		}
	}

	for _, extra := range extras {
		maps.Copy(res, extra)
	}
	return res
}
