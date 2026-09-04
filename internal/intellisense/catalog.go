package intellisense

import "github.com/unkn0wn-root/resterm/internal/directive"

// The catalog is read only after initialization. Contexts share its arguments.
type argumentCatalog map[directive.Name]args

var argTable = buildArgTable()

func argsFor(name directive.Name) args {
	return argTable[name.Canonical()]
}

func buildArgTable() argumentCatalog {
	c := make(argumentCatalog)
	addRequestArgs(c)
	addExecutionArgs(c)
	addTransportArgs(c)
	c.add(compareArgs(), directive.Compare)
	return c
}

func (c argumentCatalog) add(a args, names ...directive.Name) {
	for _, name := range names {
		name = name.Canonical()
		if _, exists := c[name]; exists {
			panic("duplicate completion arguments for " + name.Tag())
		}
		c[name] = a
	}
}
