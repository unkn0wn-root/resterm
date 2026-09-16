package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Output requires serialized calls to Append.
type Output struct {
	file     *os.File
	path     string
	document *restfile.Document
	offset   int64
}

func CreateOutput(path string) (*Output, error) {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".http" && ext != ".rest" {
		return nil, errors.New("record output must be a .http or .rest file")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open output directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	f, err := root.OpenFile(filepath.Base(path), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create recording output: %w", err)
	}
	return &Output{file: f, path: path, document: &restfile.Document{Path: path}}, nil
}

func (o *Output) Existing() *restfile.Document { return o.document }

func (o *Output) Append(ctx context.Context, p *Plan) error {
	if p.Text == "" {
		return nil
	}
	cleanup, err := p.PublishFixtures(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanup()
		}
	}()

	doc := &restfile.Document{Path: o.path, Mocks: p.Document.Mocks}
	if err := ValidateMocks(mock.Sources{Path: o.path}, doc); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := o.write(p.Text); err != nil {
		return err
	}

	o.document.Variables = append(o.document.Variables, p.Document.Variables...)
	o.document.Requests = append(o.document.Requests, p.Document.Requests...)
	o.document.Mocks = append(o.document.Mocks, p.Document.Mocks...)
	committed = true
	return nil
}

// Roll back partial writes so the file contains only complete blocks.
func (o *Output) write(text string) error {
	n, err := io.WriteString(o.file, text)
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = o.file.Sync()
	}
	if err != nil {
		return fmt.Errorf("publish recording: %w", errors.Join(err, o.rewind()))
	}
	o.offset += int64(n)
	return nil
}

func (o *Output) rewind() error {
	truncErr := o.file.Truncate(o.offset)
	_, seekErr := o.file.Seek(o.offset, io.SeekStart)
	return errors.Join(truncErr, seekErr, o.file.Sync())
}

func (o *Output) Close() error { return errors.Join(o.file.Sync(), o.file.Close()) }
