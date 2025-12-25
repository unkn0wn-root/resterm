package openapi

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Parser interface {
	Parse(ctx context.Context, path string, opts ParseOptions) (*model.Spec, error)
}

type Generator interface {
	Generate(
		ctx context.Context,
		spec *model.Spec,
		opts GeneratorOptions,
	) (*restfile.Document, error)
}

type DocumentWriter interface {
	WriteDocument(
		ctx context.Context,
		doc *restfile.Document,
		destination string,
		opts WriterOptions,
	) error
}

type ParseOptions struct {
	ResolveExternalRefs bool
}

type GeneratorOptions struct {
	BaseURLVariable      string
	IncludeDeprecated    bool
	PreferredServerIndex int
}

type WriterOptions struct {
	OverwriteExisting bool
	HeaderComment     string
}
