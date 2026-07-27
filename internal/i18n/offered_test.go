package i18n

import "testing"

func TestOfferValidation(t *testing.T) {
	cases := []struct {
		name             string
		primary, second  string
		wantP, wantS     string
		wantErr          bool
		wantSwitchHidden bool
	}{
		{name: "the usual pair", primary: "zh-CN", second: "en-US", wantP: "zh-CN", wantS: "en-US"},
		{name: "reversed for an English-first install", primary: "en-US", second: "zh-CN", wantP: "en-US", wantS: "zh-CN"},
		{name: "loose tags are normalized", primary: "zh", second: "en-GB", wantP: "zh-CN", wantS: "en-US"},
		// A single-language install is legitimate; the frontends hide the switch.
		{name: "no secondary", primary: "en-US", wantP: "en-US", wantSwitchHidden: true},
		// Unset primary must still boot.
		{name: "nothing set", wantP: Default, wantSwitchHidden: true},
		{name: "unshipped primary", primary: "fr-FR", second: "en-US", wantErr: true},
		{name: "unshipped secondary", primary: "en-US", second: "fr-FR", wantErr: true},
		// A toggle between a language and itself is a control that does nothing.
		{name: "same twice", primary: "zh-CN", second: "zh-CN", wantErr: true},
		{name: "same after normalizing", primary: "en-US", second: "en-GB", wantErr: true},
	}
	for _, c := range cases {
		got, err := Offer(c.primary, c.second)
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
		if hidden := len(got.List()) == 1; hidden != c.wantSwitchHidden {
			t.Errorf("%s: List() = %v", c.name, got.List())
		}
	}
}

func TestOfferedListIsSwitchOrder(t *testing.T) {
	o, _ := Offer("en-US", "zh-CN")
	if got := o.List(); len(got) != 2 || got[0] != "en-US" || got[1] != "zh-CN" {
		t.Errorf("List = %v, want primary first", got)
	}
}

func TestOfferedOther(t *testing.T) {
	o, _ := Offer("zh-CN", "en-US")
	if got := o.Other("zh-CN"); got != "en-US" {
		t.Errorf("Other(primary) = %q", got)
	}
	if got := o.Other("en-US"); got != "zh-CN" {
		t.Errorf("Other(secondary) = %q", got)
	}
	// An unknown current locale is on its way to the primary, so the switch
	// should offer the secondary.
	if got := o.Other("fr"); got != "en-US" {
		t.Errorf("Other(unknown) = %q", got)
	}
	single, _ := Offer("en-US", "")
	if got := single.Other("en-US"); got != "" {
		t.Errorf("a one-language install has nothing to switch to, got %q", got)
	}
}

func TestOfferedResolve(t *testing.T) {
	o, _ := Offer("zh-CN", "en-US")
	cases := []struct {
		name         string
		sess, accept string
		want         string
	}{
		{"stored wins", "en-US", "zh-CN,zh;q=0.9", "en-US"},
		{"loose stored tag", "en", "", "en-US"},
		{"falls to header", "", "en-GB,en;q=0.9", "en-US"},
		{"header the deployment does not offer", "", "fr-FR", "zh-CN"},
		{"nothing known", "", "", "zh-CN"},
	}
	for _, c := range cases {
		if got := o.Resolve(c.sess, c.accept); got != c.want {
			t.Errorf("%s: Resolve(%q,%q) = %q, want %q", c.name, c.sess, c.accept, got, c.want)
		}
	}
}

// The whole reason reads clamp instead of erroring: a session can hold a locale
// that was on offer when it was written. Changing the deployment's pair must not
// break those users' settings pages — it just puts them on the primary.
func TestOfferedResolveClampsStaleSessionLocale(t *testing.T) {
	single, _ := Offer("en-US", "")
	if got := single.Resolve("zh-CN", ""); got != "en-US" {
		t.Errorf("stale stored locale should clamp to primary, got %q", got)
	}
	if single.Has("zh-CN") {
		t.Error("Has must stay false — the write path refuses what Resolve clamps")
	}
}

// Normalize is derived from Supported rather than a switch, so adding a
// language is one list entry.
func TestNormalizeFollowsSupported(t *testing.T) {
	for _, l := range Supported {
		if got := Normalize(l); got != l {
			t.Errorf("Normalize(%q) = %q", l, got)
		}
	}
	for tag, want := range map[string]string{
		"zh-TW": "zh-CN", "ZH": "zh-CN", "en_GB": "en-US", "en-Latn-US": "en-US",
		"fr": "", "": "", "  ": "",
	} {
		if got := Normalize(tag); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", tag, got, want)
		}
	}
}
