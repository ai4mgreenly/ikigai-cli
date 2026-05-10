// Package driver consumes provider.Event streams and forwards them to
// stdout via wire.Session. Per R-VUYW-4K1X, one assistant event is
// emitted per provider turn, carrying every text, thinking, and
// tool_use block observed on the stream in arrival order.
//
// R-G0EH-D2SW: this package may import internal/provider but not any
// concrete backend subpackage.
package driver

import (
	"strings"

	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// EmitAssistantTurn drains events from the provider channel until it
// closes, then emits a single wire AssistantEvent containing the
// turn's blocks. Returns the stop reason from the EventDone if one
// was observed, or an empty string otherwise.
//
// R-4WFF-WBL4: thinking content blocks observed on the provider
// stream are forwarded as wire thinking blocks
// (R-SA9P-R1H4 shape).
func EmitAssistantTurn(s *wire.Session, events <-chan provider.Event) (string, error) {
	var blocks []any
	var textBuf strings.Builder
	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		blocks = append(blocks, wire.NewTextBlock(textBuf.String()))
		textBuf.Reset()
	}
	var stop string
	for ev := range events {
		switch e := ev.(type) {
		case provider.EventTextDelta:
			textBuf.WriteString(e.Text)
		case provider.EventThinking:
			flushText()
			blocks = append(blocks, wire.NewThinkingBlock(e.Text))
		case provider.EventToolUse:
			flushText()
			blocks = append(blocks, wire.ToolUseBlock{
				Type:  "tool_use",
				ID:    e.ID,
				Name:  e.Name,
				Input: e.Input,
			})
		case provider.EventDone:
			stop = e.StopReason
		case provider.EventUsage:
			// usage totals are not part of the v1 wire surface
			// (R-13ZB-EZZK lists them as optional).
		}
	}
	flushText()
	if err := s.EmitAssistant(wire.NewAssistantEvent(blocks...)); err != nil {
		return stop, err
	}
	return stop, nil
}
