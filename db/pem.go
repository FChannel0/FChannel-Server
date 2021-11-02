package db

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/webfinger"
)

func GetActorPemFromDB(pemID string) (activitypub.PublicKeyPem, error) {
	var pem activitypub.PublicKeyPem

	query := `select id, owner, file from publickeypem where id=$1`

	rows, err := db.Query(query, pemID)
	if err != nil {
		return pem, err
	}

	defer rows.Close()

	rows.Next()
	rows.Scan(&pem.Id, &pem.Owner, &pem.PublicKeyPem)
	f, err := os.ReadFile(pem.PublicKeyPem)
	if err != nil {
		return pem, err
	}

	pem.PublicKeyPem = strings.ReplaceAll(string(f), "\r\n", `\n`)

	return pem, nil
}

func GetActorPemFileFromDB(pemID string) (string, error) {
	query := `select file from publickeypem where id=$1`
	rows, err := db.Query(query, pemID)
	if err != nil {
		return "", err
	}

	defer rows.Close()

	var file string
	rows.Next()
	rows.Scan(&file)

	return file, nil
}

func CreatePem(actor activitypub.Actor) error {
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
		_actor, err := webfinger.FingerActor(actor.Id)
		if err != nil {
			return err
		}
		actor = _actor
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
