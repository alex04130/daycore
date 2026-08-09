package adapters

import (
	"time"

	"daycore/internal/domain"
)

// Source is one capability source after the file, the override table and the
// adapter's own manifest have been put together.
//
// # The three descriptions are three fields, on purpose
//
//	Description          the operator wrote it (providers.yaml, or the console)
//	ManifestDescription  the ADAPTER wrote it — never injected, ever
//	fallback             generated from id and capabilities when nothing is approved
//
// They are separate named fields so that no line of code can assign one to the
// other. That is the entire enforcement mechanism for the approval gate, and it
// has to be structural, because the gate has a property that makes it uniquely
// fragile: **deleting it is a one-line change that no test would catch.**
// Somebody tidying up would see two fields holding "the description", copy one
// into the other, and every adapter on the network would gain the ability to
// write into the system prompt of every conversation. Making them different
// fields with different names means that edit has to be deliberate.
//
// PromptDescription is the only accessor that injection may call, and
// TestOnlyApprovedTextReachesThePrompt asserts it never returns the manifest's.
type Source struct {
	Kind  Kind
	Entry Entry

	// Description is the operator's text, per locale. From providers.yaml, or
	// from the override row when the console has set one.
	Description map[string]string
	// Approved says the operator has read Description. Never inferred.
	Approved bool

	// ManifestDescription is what the adapter says about itself.
	//
	// ⚠️ Shown to the operator in the console as a SUGGESTION they may copy into
	// providers.yaml or the override. It is never injected, never merged into
	// Description, and there is deliberately no code path that syncs it — see
	// the type comment. If you are here to add one, that is the gate.
	ManifestDescription map[string]string

	DisplayName string
	Logo        string
	Health      *Health

	// enabled is the resolved answer: the override's opinion if it has one,
	// otherwise the file's, otherwise true.
	enabled bool
}

// Enabled reports whether this source may be used at all.
func (s *Source) Enabled() bool { return s.enabled }

// Usable is Enabled and healthy — the predicate that decides whether an id
// appears in a tool's `source` enum.
func (s *Source) Usable() bool { return s.enabled && (s.Health == nil || s.Health.Up()) }

// PromptDescription is the ONLY text from a source that may enter a prompt.
//
// Returns the operator's text when it has been approved, and otherwise a
// mechanical sentence built from facts this process already knows. Never the
// manifest's — see the Source comment for why that is a field this function
// cannot reach by accident.
func (s *Source) PromptDescription(locale string) string {
	if s.Approved {
		if t := s.Description[locale]; t != "" {
			return t
		}
		// Approved but missing this locale: fall through to the mechanical
		// sentence rather than serving another language. A description in the
		// wrong language reads as a bug in the assistant, and the loader
		// already requires both locales — reaching here means an override row
		// written by hand.
	}
	return s.mechanicalDescription(locale)
}

// mechanicalDescription is what an unapproved source gets: no third-party bytes
// at all, assembled from the id and the format.
func (s *Source) mechanicalDescription(locale string) string {
	if locale == "en-US" {
		if s.Entry.Format == FormatHTTP {
			return "external " + string(s.Kind) + " source \"" + s.Entry.ID + "\""
		}
		return string(s.Kind) + " source \"" + s.Entry.ID + "\""
	}
	if s.Entry.Format == FormatHTTP {
		return "外部" + kindZH(s.Kind) + "源「" + s.Entry.ID + "」"
	}
	return kindZH(s.Kind) + "源「" + s.Entry.ID + "」"
}

func kindZH(k Kind) string {
	switch k {
	case KindWeather:
		return "天气"
	case KindSearch:
		return "搜索"
	case KindChannel:
		return "通道"
	}
	return string(k)
}

// Resolve merges one file entry with its override row.
//
// # Why the override wins on exactly three fields and no others
//
// enabled / description / approved are read fresh at each use, so a new value
// takes effect on the next read. Everything else in Entry was used at startup
// to construct something — an HTTP client, a registered factory — and a console
// that offered to change those would be offering what the process cannot
// honour. That is the same line F1 drew across the environment variables, and
// applying it per field rather than per file is what lets one source be half
// file and half database without either half lying.
func Resolve(kind Kind, e Entry, over *domain.ProviderOverride, health *Health) *Source {
	s := &Source{
		Kind: kind, Entry: e, Health: health,
		Description: e.Description,
		DisplayName: e.ID,
		enabled:     e.Enabled == nil || *e.Enabled,
	}
	if over == nil {
		// A source described only in the file is NOT approved. Approval is "an
		// operator read this text", and putting text in a file they also edit is
		// exactly that act — but making it implicit would mean the gate has one
		// rule in the file and another in the table, and the weaker rule is the
		// one that ends up mattering. One rule: approval is an explicit row.
		return s
	}
	if over.Enabled != nil {
		s.enabled = *over.Enabled
	}
	if len(over.Description) > 0 {
		s.Description = over.Description
	}
	// Approval only counts against the text it was granted for. A stored hash
	// that no longer matches means the description was edited after approval —
	// treat it as unapproved rather than as approved-with-different-words.
	s.Approved = over.Approved && over.DescriptionHash != "" &&
		over.DescriptionHash == DescriptionHash(s.Description)
	return s
}

// AdminView is what GET /api/admin/providers reports for one source.
//
// # Boundary: no secret, and no value that stands in for one
//
// TokenEnv is the variable NAME, which is not a credential; TokenSet is whether
// that variable currently holds anything. Neither the token nor its length nor
// a prefix ever appears — a masked secret still tells an attacker how long it is
// and whether it changed between two reads, and the console needs neither to do
// its job.
type AdminView struct {
	Kind        Kind              `json:"kind"`
	ID          string            `json:"id"`
	Format      Format            `json:"format"`
	Impl        string            `json:"impl,omitempty"`
	BaseURL     string            `json:"baseUrl,omitempty"`
	TokenEnv    string            `json:"tokenEnv,omitempty"`
	TokenSet    bool              `json:"tokenSet"`
	DisplayName string            `json:"displayName"`
	Logo        string            `json:"logo,omitempty"`
	Enabled     bool              `json:"enabled"`
	Editable    []string          `json:"editable"`
	Description map[string]string `json:"description,omitempty"`
	// ManifestDescription is shown so the operator can read what the adapter
	// suggests and decide to adopt it. Labelled distinctly in the response for
	// the same reason it is a separate field in Go.
	ManifestDescription map[string]string `json:"manifestDescription,omitempty"`
	Approved            bool              `json:"approved"`
	Health              Snapshot          `json:"health"`
	// Instance names which process answered. Health is per process by design,
	// so two consoles legitimately disagree; without this there is no way to
	// tell which machine you are looking at.
	Instance string `json:"instance,omitempty"`
}

// View renders a source for the console.
func (s *Source) View(instance string) AdminView {
	v := AdminView{
		Kind: s.Kind, ID: s.Entry.ID, Format: s.Entry.Format, Impl: s.Entry.Impl,
		BaseURL: s.Entry.BaseURL, TokenEnv: s.Entry.TokenEnv, TokenSet: s.Entry.Token() != "",
		DisplayName: s.DisplayName, Logo: s.Logo,
		Enabled: s.enabled, Description: s.Description,
		ManifestDescription: s.ManifestDescription,
		Approved:            s.Approved,
		// Precomputed so the console does not re-derive a rule that lives here,
		// and so "why is this greyed out" has one answer.
		Editable: []string{"enabled", "description", "approved"},
		Instance: instance,
	}
	if s.Health != nil {
		v.Health = s.Health.Snapshot()
	} else {
		v.Health = Snapshot{Up: true, LastChecked: time.Time{}}
	}
	return v
}
