package importer

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

type FileWriter struct{}

func NewFileWriter() *FileWriter {
	return &FileWriter{}
}

func (w *FileWriter) WriteDocument(
	ctx context.Context,
	doc *restfile.Document,
	dst string,
	opts WriterOptions,
) error {
	return restwriter.WriteDocument(ctx, doc, dst, restwriter.Options{
		OverwriteExisting: opts.OverwriteExisting,
		HeaderComment:     opts.HeaderComment,
	})
}
