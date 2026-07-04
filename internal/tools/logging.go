package tools

import (
	"net/url"
	"strings"
)

// StripPassword from URL address.
func StripPassword(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}
	// Use url.Redacted, which replaces the password in the parsed userinfo before
	// re-encoding. The previous approach string-replaced the DECODED password in
	// the ENCODED URL, so a password containing a URL-special char (e.g. "@", a
	// space, or non-ASCII) never matched and was logged in the clear.
	return u.Redacted()
}

// GetLogAddresses returns a string with addresses (concatenated with comma)
// with password stripped from each address.
func GetLogAddresses(addresses []string) string {
	cleanedAddresses := make([]string, 0, len(addresses))
	for _, a := range addresses {
		cleanedAddress := StripPassword(a)
		cleanedAddresses = append(cleanedAddresses, cleanedAddress)
	}
	return strings.Join(cleanedAddresses, ", ")
}

// RedactedLogURLs prepares URLs to be logged or shown in UI stripping auth info from them.
func RedactedLogURLs(urls ...string) []string {
	var result []string

	for _, input := range urls {
		// Split the input by commas to handle comma-separated URLs.
		urlParts := strings.Split(input, ",")
		var cleanedParts []string

		for _, urlString := range urlParts {
			parsedURL, err := url.Parse(strings.TrimSpace(urlString))
			var cleanedURL string
			if err != nil {
				cleanedURL = "<invalid_url>"
			} else {
				cleanedURL = parsedURL.Redacted()
			}
			cleanedParts = append(cleanedParts, cleanedURL)
		}

		// Combine the cleaned URLs back into a comma-separated string.
		if len(cleanedParts) > 0 {
			result = append(result, strings.Join(cleanedParts, ","))
		}
	}

	return result
}
