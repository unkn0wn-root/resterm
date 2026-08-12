package rtshost

import (
	"context"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type PreRequest struct {
	Doc     *restfile.Document
	Scripts []restfile.ScriptBlock
	BaseDir string
	// Runtime is called once per executed block and must be set.
	Runtime func() (Runtime, error)
}

// RunPreRequest executes the RTS pre-request blocks of in.Scripts in document order.
func RunPreRequest(ctx context.Context, eng *Engine, in PreRequest) error {
	nth := 0
	for _, block := range in.Scripts {
		if !restfile.IsPreRequestScript(block, restfile.ScriptLangRTS) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		nth++
		src, err := loadSource(in.Doc, block, in.BaseDir)
		if err != nil {
			return fmt.Errorf("rts pre-request script %d: %w", nth, err)
		}
		if strings.TrimSpace(src.Text) == "" {
			continue
		}
		rt, err := in.Runtime()
		if err != nil {
			return src.annotate(err)
		}
		if _, err := eng.ExecModule(ctx, rt, src.Text, src.Pos); err != nil {
			return src.annotate(err)
		}
	}
	return nil
}
