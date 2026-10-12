package llm

import "context"

// Purposes of AI calls in the cost accounting.
const (
	PurposeChat          = "chat"
	PurposeCheckIn       = "check_in"
	PurposeReminder      = "reminder"
	PurposeReflection    = "reflection"
	PurposeWebSearch     = "web_search"
	PurposeTextReader    = "text_reader"
	PurposeTranscription = "transcription"
	PurposeEmbedding     = "embedding"
	PurposeOther         = "other"
)

// Payer is who an AI call is made for and why. It travels in the context, so concurrent runs never mix it up.
type Payer struct {
	UserID  int64
	Purpose string
}

type payerKey struct{}

// For sets who the calls made with ctx are for. A nested call (a tool reading a page during a reply)
// passes userID 0 and only overrides the purpose, the user stays.
func For(ctx context.Context, userID int64, purpose string) context.Context {
	if userID == 0 {
		userID = PayerOf(ctx).UserID
	}
	return context.WithValue(ctx, payerKey{}, Payer{UserID: userID, Purpose: purpose})
}

// PayerOf returns who pays for calls made with ctx, PurposeOther without a user when nobody was set.
func PayerOf(ctx context.Context) Payer {
	if p, ok := ctx.Value(payerKey{}).(Payer); ok {
		return p
	}
	return Payer{Purpose: PurposeOther}
}
