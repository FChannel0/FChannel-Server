package db

import (
	"fmt"
	"math/rand"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
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

func DeleteBoardMod(verify Verify) error {
	query := `select code from boardaccess where identifier=$1 and board=$1`

	rows, err := config.DB.Query(query, verify.Identifier, verify.Board)
	if err != nil {
		return err
	}

	defer rows.Close()

	var code string
	rows.Next()
	rows.Scan(&code)

	if code != "" {
		query := `delete from crossverification where code=$1`

		if _, err := config.DB.Exec(query, code); err != nil {
			return err
		}

		query = `delete from boardaccess where identifier=$1 and board=$2`

		if _, err := config.DB.Exec(query, verify.Identifier, verify.Board); err != nil {
			return err
		}
	}

	return nil
}

func GetBoardMod(identifier string) (Verify, error) {
	var nVerify Verify

	query := `select code, board, type, identifier from boardaccess where identifier=$1`

	rows, err := config.DB.Query(query, identifier)

	if err != nil {
		return nVerify, err
	}

	defer rows.Close()

	rows.Next()
	rows.Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier)

	return nVerify, nil
}

func CreateBoardMod(verify Verify) error {
	pass := util.CreateKey(50)

	query := `select code from verification where identifier=$1 and type=$2`

	rows, err := config.DB.Query(query, verify.Board, verify.Type)
	if err != nil {
		return err
	}

	defer rows.Close()

	var code string

	rows.Next()
	rows.Scan(&code)

	if code != "" {

		query := `select identifier from boardaccess where identifier=$1 and board=$2`

		rows, err := config.DB.Query(query, verify.Identifier, verify.Board)
		if err != nil {
			return err
		}

		defer rows.Close()

		var ident string
		rows.Next()
		rows.Scan(&ident)

		if ident != verify.Identifier {

			query := `insert into crossverification (verificationcode, code) values ($1, $2)`

			if _, err := config.DB.Exec(query, code, pass); err != nil {
				return err
			}

			query = `insert into boardaccess (identifier, code, board, type) values ($1, $2, $3, $4)`

			if _, err = config.DB.Exec(query, verify.Identifier, pass, verify.Board, verify.Type); err != nil {
				return err
			}

			fmt.Printf("Board access - Board: %s, Identifier: %s, Code: %s\n", verify.Board, verify.Identifier, pass)
		}
	}

	return nil
}

func CreateVerification(verify Verify) error {
	query := `insert into verification (type, identifier, code, created) values ($1, $2, $3, $4)`

	_, err := config.DB.Exec(query, verify.Type, verify.Identifier, verify.Code, time.Now().UTC().Format(time.RFC3339))
	return err
}

func GetVerificationByEmail(email string) (Verify, error) {
	// TODO: this only needs to select one row.

	var verify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1`

	rows, err := config.DB.Query(query, email)
	if err != nil {
		return verify, err
	}

	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
			return verify, err
		}
	}

	return verify, nil
}

func GetVerificationByCode(code string) (Verify, error) {
	// TODO: this only needs to select one row.

	var verify Verify

	query := `select type, identifier, code, board from boardaccess where code=$1`

	rows, err := config.DB.Query(query, code)
	if err != nil {
		return verify, err
	}

	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
			return verify, err
		}
	}

	return verify, nil
}

func GetVerificationCode(verify Verify) (Verify, error) {
	var nVerify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1 and board=$2`

	rows, err := config.DB.Query(query, verify.Identifier, verify.Board)
	if err != nil {
		return verify, err
	}

	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&nVerify.Type, &nVerify.Identifier, &nVerify.Code, &nVerify.Board); err != nil {
			return nVerify, err
		}

	}

	return nVerify, nil
}

func VerifyCooldownCurrent(auth string) (VerifyCooldown, error) {
	var current VerifyCooldown

	query := `select identifier, code, time from verificationcooldown where code=$1`

	rows, err := config.DB.Query(query, auth)
	if err != nil {
		query := `select identifier, code, time from verificationcooldown where identifier=$1`

		rows, err := config.DB.Query(query, auth)

		if err != nil {
			return current, err
		}

		defer rows.Close()

		for rows.Next() {
			if err := rows.Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
				return current, err
			}
		}
	} else {
		defer rows.Close()
	}

	for rows.Next() {
		if err := rows.Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
			return current, err
		}
	}

	return current, nil
}

func VerifyCooldownAdd(verify Verify) error {
	query := `insert into verficationcooldown (identifier, code) values ($1, $2)`

	_, err := config.DB.Exec(query, verify.Identifier, verify.Code)
	return err
}

func VerficationCooldown() error {
	query := `select identifier, code, time from verificationcooldown`

	rows, err := config.DB.Query(query)
	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var verify VerifyCooldown

		if err := rows.Scan(&verify.Identifier, &verify.Code, &verify.Time); err != nil {
			return err
		}

		nTime := verify.Time - 1

		query = `update set time=$1 where identifier=$2`

		if _, err := config.DB.Exec(query, nTime, verify.Identifier); err != nil {
			return err
		}

		VerficationCooldownRemove()
	}

	return nil
}

func VerficationCooldownRemove() error {
	query := `delete from verificationcooldown where time < 1`

	_, err := config.DB.Exec(query)
	return err
}

func SendVerification(verify Verify) error {
	fmt.Println("sending email")

	from := config.SiteEmail
	pass := config.SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	return smtp.SendMail(config.SiteEmailServer+":"+config.SiteEmailPort,
		smtp.PlainAuth("", from, pass, config.SiteEmailServer),
		from, []string{to}, []byte(msg))
}

func IsEmailSetup() bool {
	return config.SiteEmail != "" || config.SiteEmailPassword != "" || config.SiteEmailServer != "" || config.SiteEmailPort != ""
}

func HasAuth(code string, board string) (bool, error) {
	verify, err := GetVerificationByCode(code)
	if err != nil {
		return false, err
	}

	if res, err := HasBoardAccess(verify); err != nil && (verify.Board == config.Domain || (res && verify.Board == board)) {
		return true, nil
	} else {
		return false, err
	}

	return false, nil
}

func HasAuthCooldown(auth string) (bool, error) {
	current, err := VerifyCooldownCurrent(auth)
	if err != nil {
		return false, err
	}

	if current.Time > 0 {
		return true, nil
	}

	// fmt.Println("has auth is false")
	return false, nil
}

func GetVerify(access string) (Verify, error) {
	verify, err := GetVerificationByCode(access)
	if err != nil {
		return verify, err
	}

	if verify.Identifier == "" {
		verify, err = GetVerificationByEmail(access)
	}

	return verify, err
}

func CreateNewCaptcha() error {
	id := util.RandomID(8)
	file := "public/" + id + ".png"

	for true {
		if _, err := os.Stat("./" + file); err == nil {
			id = util.RandomID(8)
			file = "public/" + id + ".png"
		} else {
			break
		}
	}

	captcha := Captcha()

	var pattern string
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
		return err
	}

	cmd = exec.Command("convert", file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", captcha, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	rnd = fmt.Sprintf("%d", rand.Intn(24)-12)

	cmd = exec.Command("convert", file, "-rotate", rnd, "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	var verification Verify
	verification.Type = "captcha"
	verification.Code = captcha
	verification.Identifier = file

	return CreateVerification(verification)
}

func CreateBoardAccess(verify Verify) error {
	hasAccess, err := HasBoardAccess(verify)
	if err != nil {
		return err
	}

	if !hasAccess {
		query := `insert into boardaccess (identifier, board) values($1, $2)`

		_, err := config.DB.Exec(query, verify.Identifier, verify.Board)
		return err
	}

	return nil
}

func HasBoardAccess(verify Verify) (bool, error) {
	query := `select count(*) from boardaccess where identifier=$1 and board=$2`

	rows, err := config.DB.Query(query, verify.Identifier, verify.Board)
	if err != nil {
		return false, err
	}

	defer rows.Close()

	var count int

	rows.Next()
	rows.Scan(&count)

	if count > 0 {
		return true, nil
	} else {
		return false, nil
	}
}

func BoardHasAuthType(board string, auth string) (bool, error) {
	authTypes, err := activitypub.GetActorAuth(board)
	if err != nil {
		return false, err
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

func HasValidation(ctx *fiber.Ctx, actor activitypub.Actor) bool {
	id, _ := GetPasswordFromSession(ctx)

	if id == "" || (id != actor.Id && id != config.Domain) {
		//http.Redirect(w, r, "/", http.StatusSeeOther)
		return false
	}

	return true
}

func GetPasswordFromSession(r *fiber.Ctx) (string, string) {
	cookie := r.Cookies("session_token")

	parts := strings.Split(cookie, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}
