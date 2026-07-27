package i18n

import "testing"

func TestNewPairValidation(t *testing.T) {
	cases := []struct {
		name            string
		primary, second string
		wantP, wantS    string
		wantErr         bool
	}{
		{name: "the usual pair", primary: "zh-CN", second: "en-US", wantP: "zh-CN", wantS: "en-US"},
		{name: "reversed", primary: "en-US", second: "zh-CN", wantP: "en-US", wantS: "zh-CN"},
		{name: "loose tags normalize", primary: "zh", second: "en-GB", wantP: "zh-CN", wantS: "en-US"},
		// One language is a legitimate choice; the switch is hidden, not disabled.
		{name: "no secondary", primary: "en-US", wantP: "en-US"},
		{name: "nothing set", wantP: Default},
		{name: "not installed", primary: "sw-KE", second: "en-US", wantErr: true},
		// A toggle between a language and itself does nothing.
		{name: "same twice", primary: "zh-CN", second: "zh-CN", wantErr: true},
		{name: "same after normalizing", primary: "en-US", second: "en-GB", wantErr: true},
	}
	for _, c := range cases {
		got, err := NewPair(c.primary, c.second)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: want an error, got %+v", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.Primary != c.wantP || got.Secondary != c.wantS {
			t.Errorf("%s: got %+v, want {%s %s}", c.name, got, c.wantP, c.wantS)
		}
	}
}

func TestPairListIsSwitchOrder(t *testing.T) {
	p, _ := NewPair("en-US", "zh-CN")
	if got := p.List(); len(got) != 2 || got[0] != "en-US" || got[1] != "zh-CN" {
		t.Errorf("List = %v, want primary first", got)
	}
	single, _ := NewPair("en-US", "")
	if got := single.List(); len(got) != 1 {
		t.Errorf("List = %v, want one entry so the frontend hides the switch", got)
	}
}

func TestPairOther(t *testing.T) {
	p, _ := NewPair("zh-CN", "en-US")
	if got := p.Other("zh-CN"); got != "en-US" {
		t.Errorf("Other(primary) = %q", got)
	}
	if got := p.Other("en-US"); got != "zh-CN" {
		t.Errorf("Other(secondary) = %q", got)
	}
	single, _ := NewPair("en-US", "")
	if got := single.Other("en-US"); got != "" {
		t.Errorf("one language has nothing to switch to, got %q", got)
	}
}

// Most users never open the language settings, so the common path is a pair
// assembled from the deployment default.
func TestPairOrFillsFromDefault(t *testing.T) {
	def, _ := NewPair("zh-CN", "en-US")
	cases := []struct {
		name         string
		up, us       string
		wantP, wantS string
	}{
		{"chose neither", "", "", "zh-CN", "en-US"},
		{"chose both", "en-US", "zh-CN", "en-US", "zh-CN"},
		// Setting only a primary must not cost them their switch.
		{"chose only a primary", "en-US", "", "en-US", "zh-CN"},
		{"chose only a secondary", "", "en-US", "zh-CN", "en-US"},
		// Their primary happens to be the default secondary: hand them the
		// other half of the default rather than a dead switch.
		{"primary collides with default secondary", "en-US", "", "en-US", "zh-CN"},
		{"a stored value that is no longer installed", "sw-KE", "", "zh-CN", "en-US"},
	}
	for _, c := range cases {
		got := PairOr(c.up, c.us, def)
		if got.Primary != c.wantP || got.Secondary != c.wantS {
			t.Errorf("%s: got %+v, want {%s %s}", c.name, got, c.wantP, c.wantS)
		}
	}
}

// A single-language default must not manufacture a second language out of the
// user's primary.
func TestPairOrWithSingleLanguageDefault(t *testing.T) {
	def, _ := NewPair("en-US", "")
	if got := PairOr("", "", def); got.Secondary != "" {
		t.Errorf("got %+v, want no secondary", got)
	}
	if got := PairOr("en-US", "", def); got.Secondary != "" {
		t.Errorf("got %+v, want no secondary", got)
	}
}

func TestPairResolve(t *testing.T) {
	p, _ := NewPair("zh-CN", "en-US")
	cases := []struct {
		name            string
		current, accept string
		want            string
	}{
		{"what they are reading wins", "en-US", "zh-CN,zh;q=0.9", "en-US"},
		{"loose current tag", "en", "", "en-US"},
		{"falls to the header", "", "en-GB,en;q=0.9", "en-US"},
		{"header is outside their pair", "", "sw-KE", "zh-CN"},
		{"nothing known", "", "", "zh-CN"},
	}
	for _, c := range cases {
		if got := p.Resolve(c.current, c.accept); got != c.want {
			t.Errorf("%s: Resolve(%q,%q) = %q, want %q", c.name, c.current, c.accept, got, c.want)
		}
	}
}

// Reads clamp instead of erroring: a stored "currently reading" value goes
// stale the moment someone changes their pair, and their settings page still
// has to open.
func TestResolveClampsStaleCurrent(t *testing.T) {
	single, _ := NewPair("en-US", "")
	if got := single.Resolve("zh-CN", ""); got != "en-US" {
		t.Errorf("stale current should clamp to primary, got %q", got)
	}
	if single.Has("zh-CN") {
		t.Error("Has must stay false — the write path refuses what Resolve clamps")
	}
}

// Normalize follows the live catalog, not a compile-time list, so a language
// added by dropping in a file is immediately negotiable.
func TestNormalizeFollowsInstalled(t *testing.T) {
	for _, l := range Embedded {
		if got := Normalize(l); got != l {
			t.Errorf("Normalize(%q) = %q", l, got)
		}
	}
	for tag, want := range map[string]string{
		"zh-TW": "zh-CN", "ZH": "zh-CN", "en_GB": "en-US", "en-Latn-US": "en-US",
		"sw-KE": "", "": "", "  ": "",
	} {
		if got := Normalize(tag); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", tag, got, want)
		}
	}
	// Not installed here, but still a well-formed tag a pack could use.
	if Canonical("sw-KE") == "" {
		t.Error("Canonical must accept tags that are not installed")
	}
}
