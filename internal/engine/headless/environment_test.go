package headless

import "github.com/unkn0wn-root/resterm/internal/vars"

func testSelection(name string) vars.Selection {
	cat := vars.Catalog{}
	sel, _ := cat.Select(name, nil)
	return sel
}
