package agent

// FramingPrompt is the system prompt sent on every provider request.
// It orients the model as an agent inside an agentic loop so that
// tool-use fires in practice rather than the model behaving as a
// plain chatbot. R-8PF6-I8FP.
const FramingPrompt = `You are an agent running inside an automated agentic loop. ` +
	`The tools listed in this request are available for you to call. ` +
	`Use them as needed to complete the task before producing your final answer. ` +
	`When you have gathered all necessary information and are ready to respond, ` +
	`emit your final structured output.`
