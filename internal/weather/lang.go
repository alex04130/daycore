package weather

import "strings"

// Lang maps a Daycore locale onto whatever language code a particular provider
// speaks. Every upstream has its own spelling — QWeather wants "zh", the free
// wttr.in wants "zh", OpenWeatherMap wants "zh_cn" — so the table lives with the
// provider and only the lookup is shared.
//
// codes is keyed by base language ("zh", "ja"), not by full locale: an upstream
// that has one Chinese translation does not care whether the request said zh-CN
// or zh-TW, and the providers that do make regional distinctions can key on the
// full tag, which is tried first.
//
// fallback is what to send when the user's language is one the provider has
// never heard of. It is the provider's own default (usually English), not an
// error: a forecast in the wrong language still tells you it is going to rain.
func Lang(locale string, codes map[string]string, fallback string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if c, ok := codes[l]; ok {
		return c
	}
	if i := strings.IndexAny(l, "-_"); i >= 0 {
		if c, ok := codes[l[:i]]; ok {
			return c
		}
	}
	return fallback
}
