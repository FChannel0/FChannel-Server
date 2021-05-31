package main

import "strings"
import "regexp"

// False positive for application/ld+ld, application/activity+ld, application/json+json
var activityRegexp = regexp.MustCompile("application\\/(ld|json|activity)((\\+(ld|json))|$)")

func acceptActivity(header string) bool {
	accept := false
	if strings.Contains(header, ";") {
		split := strings.Split(header, ";")
		accept = accept || activityRegexp.MatchString(split[0])
		accept =  accept || strings.Contains(split[len(split)-1], "profile=\"https://www.w3.org/ns/activitystreams\"")
	} else {
		accept = accept || activityRegexp.MatchString(header)
	}
	return accept
}
