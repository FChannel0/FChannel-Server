package util

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"regexp"
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

func CreateUniqueID(actor string) (string, error) {
	var newID string
	isUnique := false
	for !isUnique {
		newID = RandomID(8)

		query := "select id from activitystream where id=$1"
		args := fmt.Sprintf("%s/%s/%s", config.Domain, actor, newID)
		rows, err := config.DB.Query(query, args)
		if err != nil {
			return "", err
		}

		defer rows.Close()

		// reusing a variable here
		// if we encounter a match, it'll get set to false causing the outer for loop to loop and to go through this all over again
		// however if nothing is there, it'll remain true and exit the loop
		isUnique = true
		for rows.Next() {
			isUnique = false
			break
		}
	}

	return newID, nil
}

func GetFileContentType(out multipart.File) (string, error) {
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	out.Seek(0, 0)

	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func GetContentType(location string) string {
	elements := strings.Split(location, ";")
	if len(elements) > 0 {
		return elements[0]
	} else {
		return location
	}
}

func CreatedNeededDirectories() {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		os.Mkdir("./public", 0755)
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		os.MkdirAll("./pem/board", 0700)
	}
}

func LoadThemes() {
	// get list of themes
	themes, err := ioutil.ReadDir("./static/css/themes")
	if err != nil {
		panic(err)
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			config.Themes = append(config.Themes, strings.TrimSuffix(f.Name(), e))
		}
	}
}

func GetBoardAuth(board string) ([]string, error) {
	var auth []string

	query := `select type from actorauth where board=$1`

	var rows *sql.Rows
	var err error
	if rows, err = config.DB.Query(query, board); err != nil {
		return auth, err
	}

	defer rows.Close()
	for rows.Next() {
		var _type string
		if err := rows.Scan(&_type); err != nil {
			return auth, err
		}

		auth = append(auth, _type)
	}

	return auth, nil
}
