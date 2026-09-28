package voice

import (
	"strings"
	"testing"
	"time"

	"github.com/tonypine/job-search-hub/server/internal/store"
)

func TestSamplesArePickedPerLanguageWithoutRepeats(t *testing.T) {
	at := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	message := func(text string) store.VoiceSample { return store.VoiceSample{SentAt: at, Content: text} }
	messages := []store.VoiceSample{
		message("Oi Ana, tudo bem? Obrigado pelo contato, estou aberto a conversar sobre a vaga."),
		message("Hi Sam, thanks for reaching out. I'd be happy to talk about the role this week."),
		message("Hi Sam, thanks for reaching out. I'd be happy to talk about the role this week."),
		message("ok 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍 👍"),
	}

	samples := PickSamples(messages)

	if len(samples) != 2 || samples[0].Language != Portuguese || samples[1].Language != English {
		t.Fatalf("samples = %+v; want one per language, the repeat and the wordless one skipped", samples)
	}
	if text := FormatSamples(samples); !strings.Contains(text, "[English, Mar 2025]") {
		t.Fatalf("formatted = %q", text)
	}
}
