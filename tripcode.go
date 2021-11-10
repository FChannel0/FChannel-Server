package main

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	_ "github.com/lib/pq"
	"github.com/simia-tech/crypt"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
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

func TripCode(pass string) (string, error) {
	pass = TripCodeConvert(pass)

	var salt [2]rune

	s := []rune(pass + "H..")[1:3]

	for i, r := range s {
		salt[i] = rune(SaltTable[r%256])
	}

	enc, err := crypt.Crypt(pass, "$1$"+string(salt[:]))
	if err != nil {
		return "", err
	}

	// normally i would just return error here but if the encrypt fails, this operation may fail and as a result cause a panic
	return enc[len(enc)-10 : len(enc)], nil
}

func TripCodeSecure(pass string) (string, error) {
	pass = TripCodeConvert(pass)

	enc, err := crypt.Crypt(pass, "$1$"+config.Salt)
	if err != nil {
		return "", err
	}

	return enc[len(enc)-10 : len(enc)], nil
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

func CreateNameTripCode(r *http.Request) (string, string, error) {
	// TODO: to allow this to compile, this will fail for the case of the admin
	// this can be easily fixed when the rest of the code gets converted to fiber

	input := r.FormValue("name")

	tripSecure := regexp.MustCompile("##(.+)?")

	if tripSecure.MatchString(input) {
		chunck := tripSecure.FindString(input)
		chunck = strings.Replace(chunck, "##", "", 1)

		//ce := regexp.MustCompile(`(?i)Admin`)
		//admin := ce.MatchString(chunck)

		//board, modcred := GetPasswordFromSession(r)

		//if admin && HasAuth(modcred, board) {
		//	return tripSecure.ReplaceAllString(input, ""), "#Admin"
		//}

		hash, err := TripCodeSecure(chunck)
		return tripSecure.ReplaceAllString(input, ""), "!!" + hash, err
	}

	trip := regexp.MustCompile("#(.+)?")

	if trip.MatchString(input) {
		chunck := trip.FindString(input)
		chunck = strings.Replace(chunck, "#", "", 1)

		//ce := regexp.MustCompile(`(?i)Admin`)
		//admin := ce.MatchString(chunck)
		//board, modcred := GetPasswordFromSession(r)

		//if admin && HasAuth(db, modcred, board) {
		//	return trip.ReplaceAllString(input, ""), "#Admin"
		//}

		hash, err := TripCode(chunck)
		return trip.ReplaceAllString(input, ""), "!" + hash, err
	}

	return input, "", nil
}
