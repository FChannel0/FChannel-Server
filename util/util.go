package util

import (
	"regexp"
)

func IsOnion(url string) bool {
	re := regexp.MustCompile(`\.onion`)
	if re.MatchString(url) {
		return true
	}

	return false
}
