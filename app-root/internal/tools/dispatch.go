package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ai4mgreenly/ikigai-cli/internal/tools/bash"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/edit"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/glob"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/grep"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/read"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/write"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// Dispatch routes a tool_use block to its implementation and returns the
// tool_result and an optional sidecar. Unknown tool names produce an
// is_error result rather than a Go error, so the caller always receives a
// correlatable answer. R-8293-8LCI.
//
// The sidecar is tool-specific (R-DPI6-73NQ): Bash returns a BashSidecar;
// tools that have no Claude Code sidecar return nil.
func Dispatch(_ context.Context, block wire.ToolUseBlock) (wire.ToolResultBlock, any, error) {
	switch block.Name {
	case bash.Name:
		var in struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Bash: invalid input: %v", err))
			return b, nil, e
		}
		result, err := bash.Run(block.ID, in.Command)
		sidecar := bash.BashSidecar{
			Stdout:      result.Stdout,
			Stderr:      result.Stderr,
			Interrupted: result.Interrupted,
		}
		return result.Block, sidecar, err
	case read.Name:
		var in struct {
			FilePath string `json:"file_path"`
			Offset   int    `json:"offset"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Read: invalid input: %v", err))
			return b, nil, e
		}
		b, e := read.Read(block.ID, in.FilePath, in.Offset, in.Limit)
		return b, nil, e
	case write.Name:
		var in struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Write: invalid input: %v", err))
			return b, nil, e
		}
		b, e := write.Write(block.ID, in.FilePath, in.Content)
		return b, nil, e
	case glob.Name:
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Glob: invalid input: %v", err))
			return b, nil, e
		}
		b, e := glob.Glob(block.ID, in.Pattern, in.Path)
		return b, nil, e
	case grep.Name:
		var in grep.Input
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Grep: invalid input: %v", err))
			return b, nil, e
		}
		b, e := grep.Grep(block.ID, in)
		return b, nil, e
	case edit.Name:
		var in struct {
			FilePath   string `json:"file_path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Edit: invalid input: %v", err))
			return b, nil, e
		}
		b, e := edit.Edit(block.ID, in.FilePath, in.OldString, in.NewString, in.ReplaceAll)
		return b, nil, e
	default:
		b, e := wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("unknown tool: %q", block.Name))
		return b, nil, e
	}
}
