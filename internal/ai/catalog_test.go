package ai

import (
	"context"
	"testing"
)

type fakeChat struct{ id string }

func (f fakeChat) Chat(context.Context, ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{}, nil
}
func (f fakeChat) ChatStream(context.Context, ChatRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk)
	close(ch)
	return ch, nil
}
func (f fakeChat) Capabilities() Capabilities { return Capabilities{} }
func (f fakeChat) Model() string              { return f.id }

// fakeWhisper is a provider that also implements Transcriber — the shape a
// whisper.cpp gateway or a major vendor's audio route would register as.
type fakeWhisper struct{ fakeChat }

func (fakeWhisper) Transcribe(context.Context, TranscriptionRequest) (Transcript, error) {
	return Transcript{Text: "ok"}, nil
}

func testCatalog(providers ...AIProvider) *Catalog {
	c := &Catalog{providers: map[string]AIProvider{}}
	for _, p := range providers {
		c.providers[p.Model()] = p
		c.order = append(c.order, p.Model())
	}
	return c
}

func TestCapabilitySelectors(t *testing.T) {
	c := testCatalog(fakeChat{"plain"}, fakeWhisper{fakeChat{"audio"}})
	if _, ok := c.Transcriber(); !ok {
		t.Fatal("Transcriber() missed a provider that implements Transcribe")
	}
	if _, ok := c.SpeechSynthesizer(); ok {
		t.Fatal("SpeechSynthesizer() found one in a catalog that has none")
	}
	if _, ok := c.ImageGenerator(); ok {
		t.Fatal("ImageGenerator() found one in a catalog that has none")
	}
	if _, ok := c.Embedder(); ok {
		t.Fatal("Embedder() found one in a catalog that has none")
	}

	// Order decides when several providers offer the same capability.
	c2 := testCatalog(fakeWhisper{fakeChat{"first"}}, fakeWhisper{fakeChat{"second"}})
	tr, ok := c2.Transcriber()
	if !ok {
		t.Fatal("Transcriber() missed both providers")
	}
	if p, ok := tr.(fakeWhisper); !ok || p.id != "first" {
		t.Fatalf("Transcriber() = %v, want the catalog-order first", tr)
	}
}
