package llm

// Every model the app calls and which one does what. These are the defaults: the admin panel (/panel/ai) can move
// any role to another provider, model or effort without a deploy. Change defaults here, not at call sites.

const (
	haiku = "anthropic/claude-haiku-5.5"
	luna  = "openai/gpt-6-luna"
)

// Roles, the keys the admin panel settings are stored under.
const (
	RoleDasyra        = "dasyra"
	RoleReflection    = "reflection"
	RoleWebSearch     = "web_search"
	RoleTextReader    = "text_reader"
	RoleTranscription = "transcription"
	RoleEmbedding     = "embedding"
	RoleCheckInGate   = "check_in_gate"
)

// Model is a model with the reasoning effort a role runs it at.
type Model struct {
	Role   string
	ID     string
	Effort string // none, low, medium, high
}

var (
	// Dasyra herself: chat, check-ins, reminders and digests. Haiku costs the same as Luna and followed the prompt
	// better on the testers' problem cases; on medium it recapped in circles and skipped memory saves, so high
	Dasyra = Model{RoleDasyra, haiku, "high"}
	// Nightly memory upkeep
	Reflection = Model{RoleReflection, haiku, "medium"}
	// Answer synthesis and evidence evaluation in web_search answer/research modes
	WebSearch = Model{RoleWebSearch, luna, "low"}
	// Cheap pass over pages and documents for web_fetch and read_document, raw text never reaches Dasyra
	TextReader = Model{RoleTextReader, luna, "low"}
	// Transcription turns voice messages, audio files and video notes into text, about $0.0002 per minute
	Transcription = Model{Role: RoleTranscription, ID: "openai/whisper-large-v3-turbo"}
	// Embedding turns memories and messages into vectors, so a memory is found by meaning and in any language:
	// about $0.02 per million tokens
	Embedding = Model{Role: RoleEmbedding, ID: "openai/text-embedding-3-small"}
	// CheckInGate asks before a check-in whether there is anything to say: a decision model answers with a probability,
	// no text, $0.02 per million input tokens. Off until turned on in the panel; only OpenRouter serves decision models
	CheckInGate = Model{Role: RoleCheckInGate, ID: "perplexity/pplx-decider-v1.1-27b"}
)

// Models are all the roles with their defaults, in the order the admin panel shows them.
var Models = []Model{Dasyra, Reflection, WebSearch, TextReader, Transcription, Embedding, CheckInGate}
