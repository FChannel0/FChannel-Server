package util

import (
	"fmt"
	"math/rand"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
)

type Verify struct {
	Type       string
	Identifier string
	Code       string
	Created    string
	Board      string
}

type VerifyCooldown struct {
	Identifier string
	Code       string
	Time       int
}

type Signature struct {
	KeyId     string
	Headers   []string
	Signature string
	Algorithm string
}

func (verify Verify) Create() error {
	query := `insert into verification (type, identifier, code, created) values ($1, $2, $3, $4)`
	_, err := config.DB.Exec(query, verify.Type, verify.Identifier, verify.Code, time.Now().UTC().Format(time.RFC3339))

	return MakeError(err, "Create")
}

func (verify Verify) CreateBoardAccess() error {
	hasAccess, err := verify.HasBoardAccess()

	if err != nil {
		return MakeError(err, "CreateBoardAccess")
	}

	if !hasAccess {
		query := `insert into boardaccess (identifier, board) values($1, $2)`
		_, err := config.DB.Exec(query, verify.Identifier, verify.Board)

		return MakeError(err, "CreateBoardAccess")
	}

	return nil
}

func (verify Verify) CreateBoardMod() error {
	var pass string
	var err error

	if pass, err = CreateKey(50); err != nil {
		return MakeError(err, "CreateBoardMod")
	}

	var code string

	query := `select code from verification where identifier=$1 and type=$2`
	if err := config.DB.QueryRow(query, verify.Board, verify.Type).Scan(&code); err != nil {
		return nil
	}

	var ident string

	query = `select identifier from boardaccess where identifier=$1 and board=$2`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&ident); err != nil {
		return nil
	}

	if ident != verify.Identifier {
		query := `insert into crossverification (verificationcode, code) values ($1, $2)`
		if _, err := config.DB.Exec(query, code, pass); err != nil {
			return MakeError(err, "CreateBoardMod")
		}

		query = `insert into boardaccess (identifier, code, board, type) values ($1, $2, $3, $4)`
		if _, err = config.DB.Exec(query, verify.Identifier, pass, verify.Board, verify.Type); err != nil {
			return MakeError(err, "CreateBoardMod")
		}

		config.Log.Printf("Board access - Board: %s, Identifier: %s, Code: %s\n", verify.Board, verify.Identifier, pass)
	}

	return nil
}

func (verify Verify) DeleteBoardMod() error {
	var code string

	query := `select code from boardaccess where identifier=$1 and board=$1`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&code); err != nil {
		return nil
	}

	query = `delete from crossverification where code=$1`
	if _, err := config.DB.Exec(query, code); err != nil {
		return MakeError(err, "DeleteBoardMod")
	}

	query = `delete from boardaccess where identifier=$1 and board=$2`
	if _, err := config.DB.Exec(query, verify.Identifier, verify.Board); err != nil {
		return MakeError(err, "DeleteBoardMod")
	}

	return nil
}

func (verify Verify) GetBoardMod() (Verify, error) {
	var nVerify Verify

	query := `select code, board, type, identifier from boardaccess where identifier=$1`
	if err := config.DB.QueryRow(query, verify.Identifier).Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier); err != nil {
		return nVerify, MakeError(err, "GetBoardMod")
	}

	return nVerify, nil
}

func (verify Verify) GetCode() (Verify, error) {
	var nVerify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1 and board=$2`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&nVerify.Type, &nVerify.Identifier, &nVerify.Code, &nVerify.Board); err != nil {
		return verify, nil
	}

	return nVerify, nil
}

func (verify Verify) HasBoardAccess() (bool, error) {
	var count int

	query := `select count(*) from boardaccess where identifier=$1 and board=$2`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&count); err != nil {
		return false, nil
	}

	return true, nil
}

func (verify Verify) SendVerification() error {
	config.Log.Println("sending email")

	from := config.SiteEmail
	pass := config.SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	err := smtp.SendMail(config.SiteEmailServer+":"+config.SiteEmailPort,
		smtp.PlainAuth("", from, pass, config.SiteEmailServer),
		from, []string{to}, []byte(msg))

	return MakeError(err, "SendVerification")
}

func (verify Verify) VerifyCooldownAdd() error {
	query := `insert into verficationcooldown (identifier, code) values ($1, $2)`
	_, err := config.DB.Exec(query, verify.Identifier, verify.Code)

	return MakeError(err, "VerifyCooldownAdd")
}

func BoardHasAuthType(board string, auth string) (bool, error) {
	authTypes, err := GetBoardAuth(board)

	if err != nil {
		return false, MakeError(err, "BoardHasAuthType")
	}

	for _, e := range authTypes {
		if e == auth {
			return true, nil
		}
	}

	return false, nil
}

func Captcha() string {
	rand.Seed(time.Now().UTC().UnixNano())
	domain := "ABEFHKMNPQRSUVWXYZ#$&"
	rng := 4
	newID := ""

	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}

	return newID
}

func CreateNewCaptcha() error {
	id := RandomID(8)
	file := "public/" + id + ".png"

	for true {
		if _, err := os.Stat("./" + file); err == nil {
			id = RandomID(8)
			file = "public/" + id + ".png"
		} else {
			break
		}
	}

	var pattern string

	captcha := Captcha()
	rnd := fmt.Sprintf("%d", rand.Intn(3))
	srnd := string(rnd)

	switch srnd {
	case "0":
		pattern = "pattern:verticalbricks"
		break

	case "1":
		pattern = "pattern:verticalsaw"
		break

	case "2":
		pattern = "pattern:hs_cross"
		break

	}

	cmd := exec.Command("convert", "-size", "200x98", pattern, "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	cmd = exec.Command("convert", file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", captcha, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	rnd = fmt.Sprintf("%d", rand.Intn(24)-12)
	cmd = exec.Command("convert", file, "-rotate", rnd, "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	var verification Verify

	verification.Type = "captcha"
	verification.Code = captcha
	verification.Identifier = file

	return verification.Create()
}

func GetRandomCaptcha() (string, error) {
	var verify string

	query := `select identifier from verification where type='captcha' order by random() limit 1`
	if err := config.DB.QueryRow(query).Scan(&verify); err != nil {
		return verify, MakeError(err, "GetRandomCaptcha")
	}

	return verify, nil
}

func GetCaptchaTotal() (int, error) {
	var count int

	query := `select count(*) from verification where type='captcha'`
	if err := config.DB.QueryRow(query).Scan(&count); err != nil {
		return count, MakeError(err, "GetCaptchaTotal")
	}

	return count, nil
}

func GetCaptchaCode(verify string) (string, error) {
	var code string

	query := `select code from verification where identifier=$1 limit 1`
	if err := config.DB.QueryRow(query, verify).Scan(&code); err != nil {
		return code, MakeError(err, "GetCaptchaCodeDB")
	}

	return code, nil
}

func DeleteCaptchaCode(verify string) error {
	query := `delete from verification where identifier=$1`
	_, err := config.DB.Exec(query, verify)

	if err != nil {
		return MakeError(err, "DeleteCaptchaCode")
	}

	err = os.Remove("./" + verify)
	return MakeError(err, "DeleteCaptchaCode")
}

func GetVerificationByCode(code string) (Verify, error) {
	// TODO: this only needs to select one row.

	var verify Verify

	query := `select type, identifier, code, board from boardaccess where code=$1`

	rows, err := config.DB.Query(query, code)
	if err != nil {
		return verify, MakeError(err, "GetVerificationByCode")
	}

	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
			return verify, MakeError(err, "GetVerificationByCode")
		}
	}

	return verify, nil
}

func GetVerificationByEmail(email string) (Verify, error) {
	var verify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1`
	if err := config.DB.QueryRow(query, email).Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
		return verify, nil
	}

	return verify, nil
}

func GetVerify(access string) (Verify, error) {
	verify, err := GetVerificationByCode(access)

	if err != nil {
		return verify, MakeError(err, "GetVerify")
	}

	if verify.Identifier == "" {
		verify, err = GetVerificationByEmail(access)
	}

	return verify, MakeError(err, "GetVerify")
}

func HasAuthCooldown(auth string) (bool, error) {
	var current VerifyCooldown
	var err error

	if current, err = VerifyCooldownCurrent(auth); err != nil {
		return false, MakeError(err, "HasAuthCooldown")
	}

	if current.Time > 0 {
		return true, nil
	}

	return false, nil
}

func HasAuth(code string, board string) (bool, error) {
	verify, err := GetVerificationByCode(code)
	if err != nil {
		return false, MakeError(err, "HasAuth")
	}

	if res, err := verify.HasBoardAccess(); err == nil && (verify.Board == config.Domain || (res && verify.Board == board)) {
		return true, nil
	} else {
		return false, MakeError(err, "HasAuth")
	}

	return false, nil
}

func IsEmailSetup() bool {
	return config.SiteEmail != "" || config.SiteEmailPassword != "" || config.SiteEmailServer != "" || config.SiteEmailPort != ""
}

func VerficationCooldown() error {
	query := `select identifier, code, time from verificationcooldown`
	rows, err := config.DB.Query(query)

	if err != nil {
		return MakeError(err, "VerficationCooldown")
	}

	defer rows.Close()
	for rows.Next() {
		var verify VerifyCooldown

		if err := rows.Scan(&verify.Identifier, &verify.Code, &verify.Time); err != nil {
			return MakeError(err, "VerficationCooldown")
		}

		nTime := verify.Time - 1
		query = `update set time=$1 where identifier=$2`

		if _, err := config.DB.Exec(query, nTime, verify.Identifier); err != nil {
			return MakeError(err, "VerficationCooldown")
		}

		VerficationCooldownRemove()
	}

	return nil
}

func VerficationCooldownRemove() error {
	query := `delete from verificationcooldown where time < 1`
	_, err := config.DB.Exec(query)

	return MakeError(err, "VerficationCooldownRemove")
}

func VerifyCooldownCurrent(auth string) (VerifyCooldown, error) {
	var current VerifyCooldown

	query := `select identifier, code, time from verificationcooldown where code=$1`
	if err := config.DB.QueryRow(query, auth).Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
		query := `select identifier, code, time from verificationcooldown where identifier=$1`
		if err := config.DB.QueryRow(query, auth).Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
			return current, nil
		}

		return current, nil
	}

	return current, nil
}

func GetPasswordFromSession(ctx *fiber.Ctx) (string, string) {
	cookie := ctx.Cookies("session_token")
	parts := strings.Split(cookie, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}

func MakeCaptchas(total int) error {
	dbtotal, err := GetCaptchaTotal()

	if err != nil {
		return MakeError(err, "MakeCaptchas")
	}

	difference := total - dbtotal

	for i := 0; i < difference; i++ {
		if err := CreateNewCaptcha(); err != nil {
			return MakeError(err, "MakeCaptchas")
		}
	}

	return nil
}
