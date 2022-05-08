package util

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
)

func IsOnion(url string) bool {
	re := regexp.MustCompile(`\.onion`)
	if re.MatchString(url) {
		return true
	}

	return false
}

func StripTransferProtocol(value string) string {
	re := regexp.MustCompile("(http://|https://)?(www.)?")
	value = re.ReplaceAllString(value, "")

	return value
}

func ShortURL(actorName string, url string) string {
	var reply string

	re := regexp.MustCompile(`.+\/`)
	actor := re.FindString(actorName)
	urlParts := strings.Split(url, "|")
	op := urlParts[0]

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

func CreateUniqueID(actor string) (string, error) {
	var newID string

	for true {
		newID = RandomID(8)
		query := "select id from activitystream where id=$1"
		args := fmt.Sprintf("%s/%s/%s", config.Domain, actor, newID)

		if err := config.DB.QueryRow(query, args); err != nil {
			break
		}
	}

	return newID, nil
}

func GetFileContentType(out multipart.File) (string, error) {
	buffer := make([]byte, 512)
	_, err := out.Read(buffer)

	if err != nil {
		return "", MakeError(err, "GetFileContentType")
	}

	out.Seek(0, 0)
	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func GetContentType(location string) string {
	elements := strings.Split(location, ";")

	if len(elements) > 0 {
		return elements[0]
	}

	return location
}

func CreatedNeededDirectories() error {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		if err = os.Mkdir("./public", 0755); err != nil {
			return MakeError(err, "CreatedNeededDirectories")
		}
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		if err = os.MkdirAll("./pem/board", 0700); err != nil {
			return MakeError(err, "CreatedNeededDirectories")
		}
	}

	return nil
}

func LoadThemes() error {
	themes, err := ioutil.ReadDir("./static/css/themes")

	if err != nil {
		MakeError(err, "LoadThemes")
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			config.Themes = append(config.Themes, strings.TrimSuffix(f.Name(), e))
		}
	}

	return nil
}

func GetBoardAuth(board string) ([]string, error) {
	var auth []string
	var rows *sql.Rows
	var err error

	query := `select type from actorauth where board=$1`
	if rows, err = config.DB.Query(query, board); err != nil {
		return auth, MakeError(err, "GetBoardAuth")
	}

	defer rows.Close()
	for rows.Next() {
		var _type string
		if err := rows.Scan(&_type); err != nil {
			return auth, MakeError(err, "GetBoardAuth")
		}

		auth = append(auth, _type)
	}

	return auth, nil
}

func MakeError(err error, msg string) error {
	if err != nil {
		_, _, line, _ := runtime.Caller(1)
		s := fmt.Sprintf("%s:%d : %s", msg, line, err.Error())
		return errors.New(s)
	}

	return nil
}
