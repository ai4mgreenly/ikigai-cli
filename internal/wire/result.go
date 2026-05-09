package wire

import "encoding/json"

// ResultEvent is the mandatory iteration terminator emitted on stdout.
//
// R-13ZB-EZZK: shape is
// {"type":"result","structured_output":<json-value>,"is_error":<bool>}.
// Optional fields (num_turns, duration_ms, total_cost_usd, usage) are
// not part of the MVP contract and are not modeled here.
type ResultEvent struct {
	Type             string          `json:"type"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	IsError          bool            `json:"is_error"`
}

// NewResultEvent fixes Type to "result" so callers cannot produce a
// malformed event by forgetting the discriminator. structuredOutput is
// marshaled to JSON; pass any value the iteration's --json-schema
// allows (for ralph-loops, {"status":"DONE"|"CONTINUE"}).
func NewResultEvent(structuredOutput any, isError bool) (ResultEvent, error) {
	raw, err := json.Marshal(structuredOutput)
	if err != nil {
		return ResultEvent{}, err
	}
	return ResultEvent{
		Type:             "result",
		StructuredOutput: raw,
		IsError:          isError,
	}, nil
}
