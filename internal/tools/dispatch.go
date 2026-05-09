package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ai4mgreenly/ikigai-cli/internal/tools/bash"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools/read"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// Dispatch routes a tool_use block to its implementation and returns the
// tool_result. Unknown tool names produce an is_error result rather than
// a Go error, so the caller always receives a correlatable answer.
// R-8293-8LCI.
func Dispatch(_ context.Context, block wire.ToolUseBlock) (wire.ToolResultBlock, error) {
	switch block.Name {
	case bash.Name:
		var in struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			return wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Bash: invalid input: %v", err))
		}
		return bash.Run(block.ID, in.Command)
	case read.Name:
		var in struct {
			FilePath string `json:"file_path"`
			Offset   int    `json:"offset"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(block.Input, &in); err != nil {
			return wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("Read: invalid input: %v", err))
		}
		return read.Read(block.ID, in.FilePath, in.Offset, in.Limit)
	default:
		return wire.NewToolResultBlock(block.ID, true, fmt.Sprintf("unknown tool: %q", block.Name))
	}
}
