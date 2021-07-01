package main

import (
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"github.com/simia-tech/crypt"
	"strings"
	"bytes"
	"regexp"
	"database/sql"
	_ "github.com/lib/pq"
	"net/http"	
)

const SaltTable = "" +
	"................................" +
	".............../0123456789ABCDEF" +
	"GABCDEFGHIJKLMNOPQRSTUVWXYZabcde" +
	"fabcdefghijklmnopqrstuvwxyz....." +
	"................................" +
	"................................" +
	"................................" +
	"................................"


func TripCode(pass string) string {

	pass = TripCodeConvert(pass)

	var salt [2]rune

	s := []rune(pass + "H..")[1:3]

	for i, r := range s {
		salt[i] = rune(SaltTable[r % 256])
	}

	enc, err := crypt.Crypt(pass, "$1$" + string(salt[:]))
	
	CheckError(err, "crypt broke")
	
	return enc[len(enc) - 10 : len(enc)]
}

func TripCodeSecure(pass string) string {

	pass = TripCodeConvert(pass)

	enc, err := crypt.Crypt(pass, "$1$" + Salt)
	
	CheckError(err, "crypt secure broke")
	
	return enc[len(enc) - 10 : len(enc)]
}

func TripCodeConvert(str string) string {

	var s bytes.Buffer
	transform.NewWriter(&s, japanese.ShiftJIS.NewEncoder()).Write([]byte(str))

	re := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)

	return re.Replace(s.String())
}

func CreateNameTripCode(r *http.Request, db *sql.DB) (string, string) {
	input := r.FormValue("name")

	tripSecure := regexp.MustCompile("##(.+)?")

	if tripSecure.MatchString(input) {
		chunck := tripSecure.FindString(input)
		chunck = strings.Replace(chunck, "##", "", 1)
		
		ce := regexp.MustCompile(`(?i)Admin`)		
		admin := ce.MatchString(chunck)
		board, modcred := GetPasswordFromSession(r)
		
		if(admin && HasAuth(db, modcred, board)) {
			return tripSecure.ReplaceAllString(input, ""), "#Admin"
		}

		hash := TripCodeSecure(chunck)
		return tripSecure.ReplaceAllString(input, ""), "!!" + hash
	}	
	
	trip := regexp.MustCompile("#(.+)?")

	if trip.MatchString(input) {
		chunck := trip.FindString(input)
		chunck = strings.Replace(chunck, "#", "", 1)
		
		ce := regexp.MustCompile(`(?i)Admin`)		
		admin := ce.MatchString(chunck)
		board, modcred := GetPasswordFromSession(r)
		
		if(admin && HasAuth(db, modcred, board)) {
			return trip.ReplaceAllString(input, ""), "#Admin"
		}

		hash := TripCode(chunck)
		return trip.ReplaceAllString(input, ""), "!" + hash
	}

	return input, ""
}
