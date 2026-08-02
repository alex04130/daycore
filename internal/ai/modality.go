package ai

import "sort"

// Modality is a kind of content a model can take in or give back.
//
// The set is deliberately the common ones rather than everything imaginable:
// this is the foundation, and a foundation that guesses at capabilities nobody
// offers yet ages worse than one that is honestly narrow. Adding one is a
// constant plus whatever serialises it.
type Modality string

const (
	ModalityText Modality = "text"
	// ModalityImage covers both directions: a photo the user sends and a picture
	// a model draws are the same kind of thing to everything between the wire
	// format and the channel that finally displays it.
	ModalityImage Modality = "image"
	// ModalityAudio likewise: a voice message in, speech out.
	ModalityAudio Modality = "audio"
	// ModalityDocument is PDF and friends. Kept separate from image because the
	// providers keep it separate — all three majors extract text *and* render
	// each page, which is strictly more than an image part can express.
	ModalityDocument Modality = "document"
	ModalityVideo    Modality = "video"
)

// Modalities is a set, stored sorted so that anything derived from it (a tool
// definition, a capability line in a prompt) is byte-stable across runs.
// Instability there silently destroys prompt caching — see docs/specs/transport.md.
type Modalities []Modality

func NewModalities(m ...Modality) Modalities {
	seen := map[Modality]bool{}
	out := make(Modalities, 0, len(m))
	for _, x := range m {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (m Modalities) Has(x Modality) bool {
	for _, v := range m {
		if v == x {
			return true
		}
	}
	return false
}

func (m Modalities) Strings() []string {
	out := make([]string, len(m))
	for i, v := range m {
		out[i] = string(v)
	}
	return out
}

// Carriage is how the bytes of a non-text part travel to the provider.
//
// Three forms rather than one because the providers genuinely differ, and
// pretending otherwise produces a capability that claims to work everywhere and
// fails silently on half of it:
//
//   - Inline: base64 in the request body. Universal, and the only option for
//     small images on most providers. Bad for a 30 MB PDF.
//   - URL: the provider fetches it. Anthropic's document blocks and OpenAI's
//     file inputs both accept one; it also lets an object-store-backed file bus
//     hand over a signed URL instead of proxying the bytes twice.
//   - FileID: the bytes were uploaded to the provider beforehand and this is
//     their handle. This is not an optimisation — it is the *only* way several
//     OpenAI-compatible Chinese gateways accept a document (upload to /v1/files,
//     then put `fileid://…` in a system message). A shape without it would
//     produce "PDF supported" that silently does nothing on those models.
type Carriage string

const (
	CarriageInline Carriage = "inline"
	CarriageURL    Carriage = "url"
	CarriageFileID Carriage = "file_id"
)

// Carriages is what a format can accept, per modality — declared by the format,
// not by models.yaml, because it is a property of our serialiser rather than of
// the model an operator picked.
type Carriages []Carriage

func (c Carriages) Has(x Carriage) bool {
	for _, v := range c {
		if v == x {
			return true
		}
	}
	return false
}

// PartCarrier is implemented by formats that can say which carriages they
// support for a modality. A format that says nothing is assumed to accept
// inline only — the same asymmetry as ToolStreamer: guessing low costs a base64
// round trip, guessing high sends a URL the provider will not fetch and the
// user sees a model that ignored their attachment.
type PartCarrier interface {
	Carries(m Modality) Carriages
}

// CarriagesFor reports how a provider can receive a modality.
func CarriagesFor(p AIProvider, m Modality) Carriages {
	pc, ok := p.(PartCarrier)
	if !ok {
		return Carriages{CarriageInline}
	}
	c := pc.Carries(m)
	if len(c) == 0 {
		return Carriages{CarriageInline}
	}
	return c
}
