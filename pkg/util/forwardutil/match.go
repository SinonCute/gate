package forwardutil

import (
	"gate/pkg/gate/db"
	"gate/pkg/gate/types"
	"regexp"
	"strings"

	"github.com/jellydator/ttlcache/v3"
)

func FindRoute(
	pattern string,
	dynCfg *types.DynamicConfig,
) (
	host string,
	endpoint *db.Endpoint,
) {
	if pattern == "" {
		return "", nil
	}

	if dynCfg == nil {
		return "", nil
	}

	for _, route := range dynCfg.RoutingRules {
		for _, host := range route.Domains {
			if MatchWithWildcards(pattern, host) {
				endpoint = dynCfg.Endpoints[route.TargetEndpointID]
				return host, endpoint
			}
		}
	}
	return "", nil
}

// MatchWithWildcards takes in two strings, s and pattern, and returns a boolean indicating whether s matches pattern.
// The pattern supports '*' (matches any sequence) and '?' (matches any single character).
func MatchWithWildcards(s, pattern string) bool {
	s = strings.ToLower(s)
	regItem := compiledRegexCache.Get(pattern) // regItem is *ttlcache.Item[string, *regexp.Regexp]
	if regItem == nil || regItem.Value() == nil {
		// This case should ideally not happen if loader works correctly and regexp compilation is successful
		// Or, the pattern was not valid for regexp.Compile
		return false
	}
	return regItem.Value().MatchString(s)
}

var compiledRegexCache = ttlcache.New[string, *regexp.Regexp](
	ttlcache.WithLoader[string, *regexp.Regexp](ttlcache.LoaderFunc[string, *regexp.Regexp](
		func(c *ttlcache.Cache[string, *regexp.Regexp], pattern string) *ttlcache.Item[string, *regexp.Regexp] {
			// Normalize pattern for regex compilation
			normalizedPattern := strings.ToLower(pattern)
			// Convert wildcard to regex: escape regex special chars, then replace wildcards
			// For simplicity, assuming pattern doesn't have other regex chars that need escaping besides what wildcards become
			regexPattern := "^" + strings.ReplaceAll(strings.ReplaceAll(normalizedPattern, "*", ".*"), "?", ".") + "$"

			reg, err := regexp.Compile(regexPattern)
			if err != nil {
				// Failed to compile regex, store nil or handle error appropriately
				// Storing nil means MatchWithWildcards will return false for this pattern
				return c.Set(pattern, nil, ttlcache.NoTTL)
			}
			return c.Set(pattern, reg, ttlcache.NoTTL)
		}),
	),
)
