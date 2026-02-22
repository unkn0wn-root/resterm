package openapi

import (
	"context"
	"errors"
	"strings"
)

const (
	errParserNotConfigured    = "openapi: parser not configured"
	errGeneratorNotConfigured = "openapi: generator not configured"
	errWriterNotConfigured    = "openapi: writer not configured"
)

type Service struct {
	Parser    Parser
	Generator Generator
	Writer    DocumentWriter
}

type GenerateOptions struct {
	Parse    ParseOptions
	Generate GeneratorOptions
	Write    WriterOptions
}

func (s *Service) GenerateHTTPFile(
	ctx context.Context,
	specPath, outputPath string,
	opts GenerateOptions,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.Parser == nil {
		return errors.New(errParserNotConfigured)
	}
	if s.Generator == nil {
		return errors.New(errGeneratorNotConfigured)
	}
	if s.Writer == nil {
		return errors.New(errWriterNotConfigured)
	}

	spec, err := s.Parser.Parse(ctx, specPath, opts.Parse)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	doc, err := s.Generator.Generate(ctx, spec, opts.Generate)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	writeOpts := opts.Write
	if warnProvider, ok := s.Parser.(Warner); ok {
		writeOpts.HeaderComment = appendWarnings(writeOpts.HeaderComment, warnProvider.Warnings())
	}
	if warnProvider, ok := s.Generator.(Warner); ok {
		writeOpts.HeaderComment = appendWarnings(writeOpts.HeaderComment, warnProvider.Warnings())
	}

	if err := s.Writer.WriteDocument(ctx, doc, outputPath, writeOpts); err != nil {
		return err
	}
	return ctx.Err()
}

func appendWarnings(base string, ws []string) string {
	if len(ws) == 0 {
		return base
	}

	var b strings.Builder
	txt := strings.TrimSpace(base)
	if txt != "" {
		b.WriteString(txt)
	}

	for _, w := range ws {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Warning: ")
		b.WriteString(w)
	}

	return b.String()
}
