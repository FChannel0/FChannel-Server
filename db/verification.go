package db

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/rand"
	"net/smtp"
	"os"
	"os/exec"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	_ "github.com/lib/pq"

	crand "crypto/rand"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
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

	rows, err := db.Query(query, verify.Identifier, verify.Board)
	if err != nil {
		return err
	}

	defer rows.Close()

	var code string
	rows.Next()
	rows.Scan(&code)

	if code != "" {
		query := `delete from crossverification where code=$1`

		if _, err := db.Exec(query, code); err != nil {
			return err
		}

		query = `delete from boardaccess where identifier=$1 and board=$2`

		if _, err := db.Exec(query, verify.Identifier, verify.Board); err != nil {
			return err
		}
	}

	return nil
}

func GetBoardMod(identifier string) (Verify, error) {
	var nVerify Verify

	query := `select code, board, type, identifier from boardaccess where identifier=$1`

	rows, err := db.Query(query, identifier)

	if err != nil {
		return nVerify, err
	}

	defer rows.Close()

	rows.Next()
	rows.Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier)

	return nVerify, nil
}

func CreateBoardMod(verify Verify) error {
	pass := CreateKey(50)

	query := `select code from verification where identifier=$1 and type=$2`

	rows, err := db.Query(query, verify.Board, verify.Type)
	if err != nil {
		return err
	}

	defer rows.Close()

	var code string

	rows.Next()
	rows.Scan(&code)

	if code != "" {

		query := `select identifier from boardaccess where identifier=$1 and board=$2`

		rows, err := db.Query(query, verify.Identifier, verify.Board)
		if err != nil {
			return err
		}

		defer rows.Close()

		var ident string
		rows.Next()
		rows.Scan(&ident)

		if ident != verify.Identifier {

			query := `insert into crossverification (verificationcode, code) values ($1, $2)`

			if _, err := db.Exec(query, code, pass); err != nil {
				return err
			}

			query = `insert into boardaccess (identifier, code, board, type) values ($1, $2, $3, $4)`

			if _, err = db.Exec(query, verify.Identifier, pass, verify.Board, verify.Type); err != nil {
				return err
			}

			fmt.Printf("Board access - Board: %s, Identifier: %s, Code: %s\n", verify.Board, verify.Identifier, pass)
		}
	}

	return nil
}

func CreateVerification(verify Verify) error {
	query := `insert into verification (type, identifier, code, created) values ($1, $2, $3, $4)`

	_, err := db.Exec(query, verify.Type, verify.Identifier, verify.Code, time.Now().UTC().Format(time.RFC3339))
	return err
}

func GetVerificationByEmail(email string) (Verify, error) {
	// TODO: this only needs to select one row.

	var verify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1`

	rows, err := db.Query(query, email)
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

	rows, err := db.Query(query, code)
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

	rows, err := db.Query(query, verify.Identifier, verify.Board)
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

	rows, err := db.Query(query, auth)
	if err != nil {
		query := `select identifier, code, time from verificationcooldown where identifier=$1`

		rows, err := db.Query(query, auth)

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

	_, err := db.Exec(query, verify.Identifier, verify.Code)
	return err
}

func VerficationCooldown() error {
	query := `select identifier, code, time from verificationcooldown`

	rows, err := db.Query(query)
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

		if _, err := db.Exec(query, nTime, verify.Identifier); err != nil {
			return err
		}

		VerficationCooldownRemove()
	}

	return nil
}

func VerficationCooldownRemove() error {
	query := `delete from verificationcooldown where time < 1`

	_, err := db.Exec(query)
	return err
}

func SendVerification(verify Verify) error {
	fmt.Println("sending email")

	from := SiteEmail
	pass := SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	return smtp.SendMail(SiteEmailServer+":"+SiteEmailPort,
		smtp.PlainAuth("", from, pass, SiteEmailServer),
		from, []string{to}, []byte(msg))
}

func IsEmailSetup() bool {
	if SiteEmail == "" {
		return false
	} else if SiteEmailPassword == "" {
		return false
	} else if SiteEmailServer == "" {
		return false
	} else if SiteEmailPort == "" {
		return false
	}

	return true
}

func HasAuth(code string, board string) (bool, error) {
	verify, err := GetVerificationByCode(code)
	if err != nil {
		return false, err
	}

	if verify.Board == Domain || (HasBoardAccess(db, verify) && verify.Board == board) {
		return true, nil
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

		_, err := db.Exec(query, verify.Identifier, verify.Board)
		return err
	}

	return nil
}

func HasBoardAccess(verify Verify) (bool, error) {
	query := `select count(*) from boardaccess where identifier=$1 and board=$2`

	rows, err := db.Query(query, verify.Identifier, verify.Board)
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
	authTypes, err := GetActorAuth(board)
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

func CreatePem(actor Actor) error {
	privatekey, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		return err
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privatekey)

	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	privatePem, err := os.Create("./pem/board/" + actor.Name + "-private.pem")
	if err != nil {
		return err
	}

	if err := pem.Encode(privatePem, privateKeyBlock); err != nil {
		return err
	}

	publickey := &privatekey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publickey)
	if err != nil {
		return err
	}

	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	publicPem, err := os.Create("./pem/board/" + actor.Name + "-public.pem")
	if err != nil {
		return err
	}

	if err := pem.Encode(publicPem, publicKeyBlock); err != nil {
		return err
	}

	_, err = os.Stat("./pem/board/" + actor.Name + "-public.pem")
	if os.IsNotExist(err) {
		return err
	} else {
		return StorePemToDB(actor)
	}

	fmt.Println(`Created PEM keypair for the "` + actor.Name + `" board. Please keep in mind that
the PEM key is crucial in identifying yourself as the legitimate owner of the board,
so DO NOT LOSE IT!!! If you lose it, YOU WILL LOSE ACCESS TO YOUR BOARD!`)

	return nil
}

func CreatePublicKeyFromPrivate(actor *activitypub.Actor, publicKeyPem string) error {
	publicFilename, err := GetActorPemFileFromDB(publicKeyPem)
	if err != nil {
		return err
	}

	privateFilename := strings.ReplaceAll(publicFilename, "public.pem", "private.pem")
	if _, err := os.Stat(privateFilename); err == nil {
		// Not a lost cause
		priv, err := ioutil.ReadFile(privateFilename)
		if err != nil {
			return err
		}

		block, _ := pem.Decode([]byte(priv))
		if block == nil || block.Type != "RSA PRIVATE KEY" {
			return errors.New("failed to decode PEM block containing public key")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}

		publicKeyDer, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return err
		}

		pubKeyBlock := pem.Block{
			Type:    "PUBLIC KEY",
			Headers: nil,
			Bytes:   publicKeyDer,
		}

		publicFileWriter, err := os.Create(publicFilename)
		if err != nil {
			return err
		}

		if err := pem.Encode(publicFileWriter, &pubKeyBlock); err != nil {
			return err
		}
	} else {
		fmt.Println(`\nUnable to locate private key from public key generation. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.Name + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return errors.New("unable to locate private key")
	}
	return nil
}

func StorePemToDB(actor activitypub.Actor) error {
	query := "select publicKeyPem from actor where id=$1"
	rows, err := db.Query(query, actor.Id)
	if err != nil {
		return err
	}

	defer rows.Close()

	var result string
	rows.Next()
	rows.Scan(&result)

	if result != "" {
		return errors.New("already storing public key for actor")
	}

	publicKeyPem := actor.Id + "#main-key"
	query = "update actor set publicKeyPem=$1 where id=$2"
	if _, err := db.Exec(query, publicKeyPem, actor.Id); err != nil {
		return err
	}

	file := "./pem/board/" + actor.Name + "-public.pem"
	query = "insert into publicKeyPem (id, owner, file) values($1, $2, $3)"
	_, err = db.Exec(query, publicKeyPem, actor.Id, file)
	return err
}

func ActivitySign(actor activitypub.Actor, signature string) (string, error) {
	query := `select file from publicKeyPem where id=$1 `

	rows, err := db.Query(query, actor.PublicKey.Id)
	if err != nil {
		return "", err
	}

	defer rows.Close()

	var file string
	rows.Next()
	rows.Scan(&file)

	file = strings.ReplaceAll(file, "public.pem", "private.pem")
	_, err = os.Stat(file)
	if err != nil {
		fmt.Println(`\n Unable to locate private key. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.Name + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return "", errors.New("unable to locate private key")
	}

	publickey, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(publickey)

	pub, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	rng := crand.Reader
	hashed := sha256.New()
	hashed.Write([]byte(signature))
	cipher, _ := rsa.SignPKCS1v15(rng, pub, crypto.SHA256, hashed.Sum(nil))

	return base64.StdEncoding.EncodeToString(cipher), nil
}

func ActivityVerify(actor activitypub.Actor, signature string, verify string) error {
	sig, _ := base64.StdEncoding.DecodeString(signature)

	if actor.PublicKey.PublicKeyPem == "" {
		actor = FingerActor(actor.Id)
	}

	block, _ := pem.Decode([]byte(actor.PublicKey.PublicKeyPem))
	pub, _ := x509.ParsePKIXPublicKey(block.Bytes)

	hashed := sha256.New()
	hashed.Write([]byte(verify))

	return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hashed.Sum(nil), sig)
}

func VerifyHeaderSignature(r *http.Request, actor activitypub.Actor) bool {
	s := ParseHeaderSignature(r.Header.Get("Signature"))

	var method string
	var path string
	var host string
	var date string
	var digest string
	var contentLength string

	var sig string
	for i, e := range s.Headers {
		var nl string
		if i < len(s.Headers)-1 {
			nl = "\n"
		}

		switch e {
		case "(request-target)":
			method = strings.ToLower(r.Method)
			path = r.URL.Path
			sig += "(request-target): " + method + " " + path + "" + nl
			break
		case "host":
			host = r.Host
			sig += "host: " + host + "" + nl
			break
		case "date":
			date = r.Header.Get("date")
			sig += "date: " + date + "" + nl
			break
		case "digest":
			digest = r.Header.Get("digest")
			sig += "digest: " + digest + "" + nl
			break
		case "content-length":
			contentLength = r.Header.Get("content-length")
			sig += "content-length: " + contentLength + "" + nl
			break
		}
	}

	if s.KeyId != actor.PublicKey.Id {
		return false
	}

	t, _ := time.Parse(time.RFC1123, date)

	if time.Now().UTC().Sub(t).Seconds() > 75 {
		return false
	}

	if ActivityVerify(actor, s.Signature, sig) != nil {
		return false
	}

	return true
}

func ParseHeaderSignature(signature string) Signature {
	var nsig Signature

	keyId := regexp.MustCompile(`keyId=`)
	headers := regexp.MustCompile(`headers=`)
	sig := regexp.MustCompile(`signature=`)
	algo := regexp.MustCompile(`algorithm=`)

	signature = strings.ReplaceAll(signature, "\"", "")
	parts := strings.Split(signature, ",")

	for _, e := range parts {
		if keyId.MatchString(e) {
			nsig.KeyId = keyId.ReplaceAllString(e, "")
			continue
		}

		if headers.MatchString(e) {
			header := headers.ReplaceAllString(e, "")
			nsig.Headers = strings.Split(header, " ")
			continue
		}

		if sig.MatchString(e) {
			nsig.Signature = sig.ReplaceAllString(e, "")
			continue
		}

		if algo.MatchString(e) {
			nsig.Algorithm = algo.ReplaceAllString(e, "")
			continue
		}
	}

	return nsig
}
