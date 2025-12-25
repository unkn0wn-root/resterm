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
	if warnProvider, ok := s.Generator.(interface{ Warnings() []string }); ok {
		warnings := warnProvider.Warnings()
		if len(warnings) > 0 {
			var builder strings.Builder
			existing := strings.TrimSpace(writeOpts.HeaderComment)
			if existing != "" {
				builder.WriteString(existing)
				builder.WriteString("\n")
			}
			for _, warning := range warnings {
				trimmed := strings.TrimSpace(warning)
				if trimmed == "" {
					continue
				}
				builder.WriteString("Warning: ")
				builder.WriteString(trimmed)
				builder.WriteString("\n")
			}
			writeOpts.HeaderComment = strings.TrimRight(builder.String(), "\n")
		}
	}

	if err := s.Writer.WriteDocument(ctx, doc, outputPath, writeOpts); err != nil {
		return err
	}
	return ctx.Err()
}
