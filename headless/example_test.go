package headless_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/headless"
)

func ExampleRun() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir, err := os.MkdirTemp("", "headless-example-*")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	rep, err := headless.Run(context.Background(), headless.Options{FilePath: path})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(rep.Passed, rep.Failed)
	// Output:
	// 1 0
}
