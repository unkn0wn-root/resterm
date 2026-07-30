package request

import "github.com/unkn0wn-root/resterm/internal/vars"

func testEnv(name string) vars.Environment {
	if name == "" {
		env, _ := (vars.Catalog{}).Resolve(vars.Selection{})
		return env
	}
	cat, _ := vars.NewCatalog(vars.EnvironmentSet{name: {}})
	sel, _ := cat.Select(name, nil)
	env, _ := cat.Resolve(sel)
	return env
}

func testConfig(name string) (vars.Catalog, vars.Selection) {
	cat, _ := vars.NewCatalog(vars.EnvironmentSet{name: {}})
	sel, _ := cat.Select(name, nil)
	return cat, sel
}
