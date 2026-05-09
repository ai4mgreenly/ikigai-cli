package wire

// R-TSL0-SJTK: every event object has a top-level `type` string that
// discriminates its shape. The v1 emitted set is {assistant, user,
// result}. EmittedTypes is the closed set ikigai-cli is allowed to
// write to stdout in v1.
var EmittedTypes = []string{"assistant", "user", "result"}

// AssistantMessage is the `message` payload of an AssistantEvent.
//
// R-VUYW-4K1X: shape is {"role":"assistant","content":[<blocks>]}.
// Role is fixed by NewAssistantEvent so callers cannot forge a
// malformed event. Content blocks are left as `any` here because
// individual block shapes are pinned by their own requirements
// (R-WQOA-2LBZ text, R-XCMG-YGOH tool_use, R-SA9P-R1H4 thinking).
type AssistantMessage struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}

// AssistantEvent carries a model turn.
type AssistantEvent struct {
	Type    string           `json:"type"`
	Message AssistantMessage `json:"message"`
}

// NewAssistantEvent fixes Type to "assistant" and Message.Role to
// "assistant" so callers cannot produce a malformed event by
// forgetting either discriminator. Content is always serialized as a
// JSON array, even when empty.
func NewAssistantEvent(content ...any) AssistantEvent {
	if content == nil {
		content = []any{}
	}
	return AssistantEvent{
		Type:    "assistant",
		Message: AssistantMessage{Role: "assistant", Content: content},
	}
}

// UserMessage is the `message` payload of a UserEvent.
//
// R-YLQR-3Z46: shape is {"role":"user","content":[<blocks>]}. Role is
// fixed by NewUserEvent so callers cannot forge a malformed event.
// Content blocks are left as `any`; individual block shapes (e.g.
// tool_result) are pinned by their own requirements.
type UserMessage struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}

// UserEvent carries either a stdin user prompt replay or a
// tool_result envelope.
type UserEvent struct {
	Type    string      `json:"type"`
	Message UserMessage `json:"message"`
}

// NewUserEvent fixes Type to "user" and Message.Role to "user" so
// callers cannot produce a malformed event by forgetting either
// discriminator. Content is always serialized as a JSON array, even
// when empty.
func NewUserEvent(content ...any) UserEvent {
	if content == nil {
		content = []any{}
	}
	return UserEvent{
		Type:    "user",
		Message: UserMessage{Role: "user", Content: content},
	}
}
