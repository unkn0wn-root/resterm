package restfile

// CloneScriptBlocks returns a copy of script block metadata.
func CloneScriptBlocks(src []ScriptBlock) []ScriptBlock {
	if len(src) == 0 {
		return nil
	}
	dst := append([]ScriptBlock(nil), src...)
	for i := range dst {
		dst[i].Lines = append([]ScriptLine(nil), src[i].Lines...)
	}
	return dst
}
