package post

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
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

func CreateNameTripCode(ctx *fiber.Ctx) (string, string, error) {
	input := ctx.FormValue("name")
	tripSecure := regexp.MustCompile("##(.+)?")

	if tripSecure.MatchString(input) {
		chunck := tripSecure.FindString(input)
		chunck = strings.Replace(chunck, "##", "", 1)
		ce := regexp.MustCompile(`(?i)Admin`)
		admin := ce.MatchString(chunck)
		board, modcred := util.GetPasswordFromSession(ctx)

		if hasAuth, _ := util.HasAuth(modcred, board); hasAuth && admin {
			return tripSecure.ReplaceAllString(input, ""), "#Admin", nil
		}

		hash, err := TripCodeSecure(chunck)

		return tripSecure.ReplaceAllString(input, ""), "!!" + hash, util.MakeError(err, "CreateNameTripCode")
	}

	trip := regexp.MustCompile("#(.+)?")

	if trip.MatchString(input) {
		chunck := trip.FindString(input)
		chunck = strings.Replace(chunck, "#", "", 1)
		ce := regexp.MustCompile(`(?i)Admin`)
		admin := ce.MatchString(chunck)
		board, modcred := util.GetPasswordFromSession(ctx)

		if hasAuth, _ := util.HasAuth(modcred, board); hasAuth && admin {
			return trip.ReplaceAllString(input, ""), "#Admin", nil
		}

		hash, err := TripCode(chunck)
		return trip.ReplaceAllString(input, ""), "!" + hash, util.MakeError(err, "CreateNameTripCode")
	}

	return input, "", nil
}

func TripCode(pass string) (string, error) {
	var salt [2]rune

	pass = TripCodeConvert(pass)
	s := []rune(pass + "H..")[1:3]

	for i, r := range s {
		salt[i] = rune(SaltTable[r%256])
	}

	enc, err := crypt.Crypt(pass, "$1$"+string(salt[:]))

	if err != nil {
		return "", util.MakeError(err, "TripCode")
	}

	// normally i would just return error here but if the encrypt fails, this operation may fail and as a result cause a panic
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

func TripCodeSecure(pass string) (string, error) {
	pass = TripCodeConvert(pass)
	enc, err := crypt.Crypt(pass, "$1$"+config.Salt)

	if err != nil {
		return "", util.MakeError(err, "TripCodeSecure")
	}

	return enc[len(enc)-10 : len(enc)], nil
}
