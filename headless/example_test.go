package headless_test

import (
	"bytes"
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
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	opts := headless.Options{FilePath: path}
	if err := opts.Validate(); err != nil {
		fmt.Println(err)
		return
	}

	rep, err := headless.Run(context.Background(), opts)
	if err != nil {
		fmt.Println(err)
		return
	}

	var out bytes.Buffer
	if err := rep.Encode(&out, headless.Text); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(rep.Passed, rep.Failed)
	// Output:
	// 1 0
}
