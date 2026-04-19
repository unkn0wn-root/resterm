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
	err = os.WriteFile(path, []byte(src), 0o600)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Run is the simplest entrypoint when you only need a single execution.
	rep, err := headless.Run(context.Background(), headless.Options{
		Source: headless.Source{Path: path},
	})
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

func ExampleRunPlan() {
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
	err = os.WriteFile(path, []byte(src), 0o600)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Build once when you want to reuse the same validated input across
	// repeated runs, retries, or concurrent callers.
	pl, err := headless.Build(headless.Options{
		Source: headless.Source{Path: path},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	rep1, err := headless.RunPlan(context.Background(), pl)
	if err != nil {
		fmt.Println(err)
		return
	}
	rep2, err := headless.RunPlan(context.Background(), pl)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(rep1.Passed, rep2.Passed)
	// Output:
	// 1 1
}
