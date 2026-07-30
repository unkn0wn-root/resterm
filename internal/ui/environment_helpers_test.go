package ui

import "github.com/unkn0wn-root/resterm/internal/vars"

func testCatalog(set vars.EnvironmentSet) vars.Catalog {
	cat, _ := vars.NewCatalog(set)
	return cat
}

func testSelection(name string) vars.Selection {
	cat := vars.Catalog{}
	sel, _ := cat.Select(name, nil)
	return sel
}

func testEnv(name string) vars.Environment {
	cat := vars.Catalog{}
	sel, _ := cat.Select(name, nil)
	env, _ := cat.Resolve(sel)
	return env
}
