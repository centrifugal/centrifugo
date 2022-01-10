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
	pass, passSet := u.User.Password()
	if passSet {
		return strings.Replace(u.String(), pass+"@", "***@", 1)
	}
	return u.String()
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
