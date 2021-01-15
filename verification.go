package main

import "fmt"
import "database/sql"
import _ "github.com/lib/pq"
import	"net/smtp"
import "time"
import "os/exec"
import "os"
import "math/rand"

type Verify struct {
	Type string
	Identifier string
	Code string
	Created string
	Board string
}

type VerifyCooldown struct {
	Identifier string
	Code string
	Time int
}

func DeleteBoardMod(db *sql.DB, verify Verify) {
	query := fmt.Sprintf("select code from boardaccess where identifier='%s' and board='%s'", verify.Identifier, verify.Board)

	rows, err := db.Query(query)

	CheckError(err, "could not select code from boardaccess")

	defer rows.Close()

	var code string
	rows.Next()
	rows.Scan(&code)

	if code != "" {
		query := fmt.Sprintf("delete from crossverification where code='%s'", code)


		_, err := db.Exec(query)
		
		CheckError(err, "could not delete code from crossverification")

		query = fmt.Sprintf("delete from boardaccess where identifier='%s' and board='%s'", verify.Identifier, verify.Board)

		_, err = db.Exec(query)
		
		CheckError(err, "could not delete identifier from boardaccess")				
	}
}

func GetBoardMod(db *sql.DB, identifier string) Verify{
	var nVerify Verify

	query := fmt.Sprintf("select code, board, type, identifier from boardaccess where identifier='%s'", identifier)

	rows, err := db.Query(query)

	CheckError(err, "could not select boardaccess query")

	defer rows.Close()

	rows.Next()
	rows.Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier)

	return nVerify
}

func CreateBoardMod(db *sql.DB, verify Verify) {
	pass := CreateKey(50)

	query := fmt.Sprintf("select code from verification where identifier='%s' and type='%s'", verify.Board, verify.Type)

	rows, err := db.Query(query)

	CheckError(err, "could not select verifcaiton query")

	defer rows.Close()

	var code string
	
	rows.Next()
	rows.Scan(&code)

	if code != "" {

		query := fmt.Sprintf("select identifier from boardaccess where identifier='%s' and board='%s'", verify.Identifier, verify.Board)

		rows, err := db.Query(query)
		
		CheckError(err, "could not select idenifier from boardaccess")

		defer rows.Close()

		var ident string
		rows.Next()
		rows.Scan(&ident)

		if ident != verify.Identifier {

			query := fmt.Sprintf("insert into crossverification (verificationcode, code) values ('%s', '%s')", code, pass)

			_, err := db.Exec(query)
			
			CheckError(err, "could not insert new crossverification")

			query = fmt.Sprintf("insert into boardaccess (identifier, code, board, type) values ('%s', '%s', '%s', '%s')", verify.Identifier, pass, verify.Board, verify.Type)

			_, err = db.Exec(query)
			
			CheckError(err, "could not insert new boardaccess")

			fmt.Printf("Board access - Board: %s, Identifier: %s, Code: %s\n", verify.Board, verify.Identifier, pass)
		}
	}
}

func CreateVerification(db *sql.DB, verify Verify) {
	query := fmt.Sprintf("insert into verification (type, identifier, code, created) values ('%s', '%s', '%s', '%s') ", verify.Type, verify.Identifier, verify.Code, time.Now().Format(time.RFC3339))

	_, err := db.Exec(query)

	CheckError(err, "error creating verify")
}

func GetVerificationByEmail(db *sql.DB, email string) Verify {
	var verify Verify

	query := fmt.Sprintf("select type, identifier, code, board from boardaccess where identifier='%s';", email)

	rows, err := db.Query(query)

	defer rows.Close()

	CheckError(err, "error getting verify by email query")		

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board)

		CheckError(err, "error getting verify by email scan")				
	}
	
	return verify
}

func GetVerificationByCode(db *sql.DB, code string) Verify {
	var verify Verify

	query := fmt.Sprintf("select type, identifier, code, board from boardaccess where code='%s';", code)

	rows, err := db.Query(query)

	defer rows.Close()

	if err != nil {
		CheckError(err, "error getting verify by code query")
		return verify
	}

	for rows.Next() {
		err := rows.Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board)

		CheckError(err, "error getting verify by code scan")				
	}
	
	return verify
}

func VerifyCooldownCurrent(db *sql.DB, auth string) VerifyCooldown {
	var current VerifyCooldown
	
	query := fmt.Sprintf("select identifier, code, time from verificationcooldown where code='%s'", auth)

	rows, err := db.Query(query)

	defer rows.Close()	

	if err != nil {

		query := fmt.Sprintf("select identifier, code, time from verificationcooldown where identifier='%s'", auth)

		rows, err := db.Query(query)

		defer rows.Close()
		
		if err != nil {
			return current
		}
		
		defer rows.Close()

		for rows.Next() {
			err = rows.Scan(&current.Identifier, &current.Code, &current.Time)

			CheckError(err, "error scanning current verify cooldown verification")
		}		
	}

	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&current.Identifier, &current.Code, &current.Time)

		CheckError(err, "error scanning current verify cooldown code")
	}

	return current
}

func VerifyCooldownAdd(db *sql.DB, verify Verify) {
	query := fmt.Sprintf("insert into verficationcooldown (identifier, code) values ('%s', '%s');", verify.Identifier, verify.Code)

	_, err := db.Exec(query)

	CheckError(err, "error adding verify to cooldown")
}

func VerficationCooldown(db *sql.DB) {
	
	query := fmt.Sprintf("select identifier, code, time from verificationcooldown")

	rows, err := db.Query(query)

	defer rows.Close()	

	CheckError(err, "error with verifiy cooldown query ")

	defer rows.Close()

	for rows.Next() {
		var verify VerifyCooldown		
		err = rows.Scan(&verify.Identifier, &verify.Code, &verify.Time)

		CheckError(err, "error with verifiy cooldown scan ")

		nTime := verify.Time - 1;

		query = fmt.Sprintf("update set time='%s' where identifier='%s'", nTime, verify.Identifier)

		_, err := db.Exec(query)

		CheckError(err, "error with update cooldown query")

		VerficationCooldownRemove(db)
	}
}

func VerficationCooldownRemove(db *sql.DB) {
	query := fmt.Sprintf("delete from verificationcooldown where time < 1;")

	_, err := db.Exec(query)

	CheckError(err, "error with verifiy cooldown remove query ")
}

func SendVerification(verify Verify) {

	fmt.Println("sending email")

	from := SiteEmail
	pass := SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	err := smtp.SendMail(SiteEmailServer + ":" + SiteEmailPort,
		smtp.PlainAuth("", from, pass, SiteEmailServer),
		from, []string{to}, []byte(msg))


	CheckError(err, "error with smtp")
}

func IsEmailSetup() bool {
	if SiteEmail == "" {
		return false
	}

	if SiteEmailPassword == "" {
		return false
	}

	if SiteEmailServer == "" {
		return false
	}

	if SiteEmailPort == "" {
		return false
	}	
	
	return true
}

func HasAuth(db *sql.DB, code string, board string) bool {

	verify := GetVerificationByCode(db, code)

	if verify.Board == Domain || (HasBoardAccess(db, verify) && verify.Board == board) {
		return true
	}

	fmt.Println("has auth is false")
	
	return false;
}

func HasAuthCooldown(db *sql.DB, auth string) bool {
	current := VerifyCooldownCurrent(db, auth)
	if current.Time > 0 {
		return true
	}

	fmt.Println("has auth is false")	
	return false
}

func GetVerify(db *sql.DB, access string) Verify {
	verify := GetVerificationByCode(db, access)

	if verify.Identifier == "" {
		verify = GetVerificationByEmail(db, access)
	}

	return verify
}

func CreateNewCaptcha(db *sql.DB){
	id   := RandomID(8)
	file := "public/" + id + ".png"
	
	for true {
		if _, err := os.Stat("./" + file); err == nil {
			id   = RandomID(8)			
			file = "public/" + id + ".png"
		}else{
			break
		}
	}
	
	captcha := Captcha()

	var pattern string
	rnd := fmt.Sprintf("%d", rand.Intn(3))

	srnd := string(rnd)

	switch srnd {
	case "0" :
		pattern = "pattern:verticalbricks"
		break

	case "1" :
		pattern = "pattern:verticalsaw"
		break
		
	case "2" :
		pattern = "pattern:hs_cross"
		break		

	}
	
	cmd := exec.Command("convert", "-size", "200x98", pattern, "-transparent", "white", file)

	err := cmd.Run()

	CheckError(err, "error with captcha first pass")
	
	cmd  = exec.Command("convert", file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", captcha, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file)

	err = cmd.Run()

	CheckError(err, "error with captcha second pass")

	rnd = fmt.Sprintf("%d", rand.Intn(24) - 12)
	
	cmd  = exec.Command("convert", file, "-rotate", rnd, "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file)

	err = cmd.Run()

	CheckError(err, "error with captcha third pass")	

	var verification Verify
	verification.Type         = "captcha"	
	verification.Code         = captcha
	verification.Identifier = file

	CreateVerification(db, verification)
}

func CreateBoardAccess(db *sql.DB, verify Verify) {
	if(!HasBoardAccess(db, verify)){
		query  := fmt.Sprintf("insert into boardaccess (identifier, board) values('%s', '%s')",
			                     verify.Identifier, verify.Board)
		
		_, err := db.Exec(query)

		CheckError(err, "could not instert verification and board into board access")
	}
}

func HasBoardAccess(db *sql.DB, verify Verify) bool {
	query := fmt.Sprintf("select count(*) from boardaccess where identifier='%s' and board='%s'",
		verify.Identifier, verify.Board)

	rows, err := db.Query(query)

	defer rows.Close()	

	CheckError(err, "could not select boardaccess based on verify")	

	var count int

	rows.Next()
	rows.Scan(&count)

	if(count > 0) {
		return true
	} else {
		return false
	}
}

func BoardHasAuthType(db *sql.DB, board string, auth string) bool {
	authTypes := GetActorAuth(db, board)

	for _, e := range authTypes {
		if(e == auth){
			return true
		}
	}
	
	return false
}

func Captcha() string {
	rand.Seed(time.Now().UnixNano())
	domain := "ABEFHKMNPQRSUVWXYZ#$&"
	rng := 4
	newID := ""
	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}
	
	return newID
}	


