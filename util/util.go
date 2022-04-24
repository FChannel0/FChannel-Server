package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func IsOnion(url string) bool {
	re := regexp.MustCompile(`\.onion`)
	if re.MatchString(url) {
		return true
	}

	return false
}

func GetActorInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if atFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)(/|\s+|\r|\r\n)?$`)
	mainActor := re.MatchString(path)
	if mainActor {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return "main", match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if httpFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 3 {
			if match[4] == "users" {
				return match[6], match[3]
			}

			return match[4], match[3]
		}
	}

	return "", ""
}

func GetActorFollowNameFromPath(path string) string {
	var actor string

	re := regexp.MustCompile("f\\w+-")

	actor = re.FindString(path)

	actor = strings.Replace(actor, "f", "", 1)
	actor = strings.Replace(actor, "-", "", 1)

	return actor
}

func StripTransferProtocol(value string) string {
	re := regexp.MustCompile("(http://|https://)?(www.)?")

	value = re.ReplaceAllString(value, "")

	return value
}

func GetCaptchaCode(captcha string) string {
	re := regexp.MustCompile("\\w+\\.\\w+$")
	code := re.FindString(captcha)

	re = regexp.MustCompile("\\w+")
	code = re.FindString(code)

	return code
}

func ShortURL(actorName string, url string) string {

	re := regexp.MustCompile(`.+\/`)

	actor := re.FindString(actorName)

	urlParts := strings.Split(url, "|")

	op := urlParts[0]

	var reply string

	if len(urlParts) > 1 {
		reply = urlParts[1]
	}

	re = regexp.MustCompile(`\w+$`)
	temp := re.ReplaceAllString(op, "")

	if temp == actor {
		id := LocalShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + LocalShort(reply)
		} else if reply != "" {
			id = id + "#" + RemoteShort(reply)
		}

		return id
	} else {
		id := RemoteShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + LocalShort(reply)
		} else if reply != "" {
			id = id + "#" + RemoteShort(reply)
		}

		return id
	}
}

func LocalShort(url string) string {
	re := regexp.MustCompile(`\w+$`)
	return re.FindString(StripTransferProtocol(url))
}

func RemoteShort(url string) string {
	re := regexp.MustCompile(`\w+$`)

	id := re.FindString(StripTransferProtocol(url))

	re = regexp.MustCompile(`.+/.+/`)

	actorurl := re.FindString(StripTransferProtocol(url))

	re = regexp.MustCompile(`/.+/`)

	actorname := re.FindString(actorurl)

	actorname = strings.Replace(actorname, "/", "", -1)

	return "f" + actorname + "-" + id
}

func ShortImg(url string) string {
	nURL := url

	re := regexp.MustCompile(`(\.\w+$)`)

	fileName := re.ReplaceAllString(url, "")

	if len(fileName) > 26 {
		re := regexp.MustCompile(`(^.{26})`)

		match := re.FindStringSubmatch(fileName)

		if len(match) > 0 {
			nURL = match[0]
		}

		re = regexp.MustCompile(`(\..+$)`)

		match = re.FindStringSubmatch(url)

		if len(match) > 0 {
			nURL = nURL + "(...)" + match[0]
		}
	}

	return nURL
}

func ConvertSize(size int64) string {
	var rValue string

	convert := float32(size) / 1024.0

	if convert > 1024 {
		convert = convert / 1024.0
		rValue = fmt.Sprintf("%.2f MB", convert)
	} else {
		rValue = fmt.Sprintf("%.2f KB", convert)
	}

	return rValue
}

// IsInStringArray looks for a string in a string array and returns true if it is found.
func IsInStringArray(haystack []string, needle string) bool {
	for _, e := range haystack {
		if e == needle {
			return true
		}
	}
	return false
}

// GetUniqueFilename will look for an available random filename in the /public/ directory.
func GetUniqueFilename(ext string) string {
	id := RandomID(8)
	file := "/public/" + id + "." + ext

	for true {
		if _, err := os.Stat("." + file); err == nil {
			id = RandomID(8)
			file = "/public/" + id + "." + ext
		} else {
			return "/public/" + id + "." + ext
		}
	}

	return ""
}

func HashMedia(media string) string {
	h := sha256.New()
	h.Write([]byte(media))
	return hex.EncodeToString(h.Sum(nil))
}

func HashBytes(media []byte) string {
	h := sha256.New()
	h.Write(media)
	return hex.EncodeToString(h.Sum(nil))
}

func EscapeString(text string) string {
	// TODO: not enough

	text = strings.Replace(text, "<", "&lt;", -1)
	return text
}
