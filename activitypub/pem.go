package activitypub

import (
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

type Signature struct {
	KeyId     string
	Headers   []string
	Signature string
	Algorithm string
}

func CreatePem(actor Actor) error {
	privatekey, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		return util.MakeError(err, "CreatePem")
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privatekey)

	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	privatePem, err := os.Create("./pem/board/" + actor.Name + "-private.pem")
	if err != nil {
		return util.MakeError(err, "CreatePem")
	}

	if err := pem.Encode(privatePem, privateKeyBlock); err != nil {
		return util.MakeError(err, "CreatePem")
	}

	publickey := &privatekey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publickey)
	if err != nil {
		return util.MakeError(err, "CreatePem")
	}

	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	publicPem, err := os.Create("./pem/board/" + actor.Name + "-public.pem")
	if err != nil {
		return util.MakeError(err, "CreatePem")
	}

	if err := pem.Encode(publicPem, publicKeyBlock); err != nil {
		return util.MakeError(err, "CreatePem")
	}

	_, err = os.Stat("./pem/board/" + actor.Name + "-public.pem")
	if os.IsNotExist(err) {
		return util.MakeError(err, "CreatePem")
	} else {
		return StorePemToDB(actor)
	}

	fmt.Println(`Created PEM keypair for the "` + actor.Name + `" board. Please keep in mind that
the PEM key is crucial in identifying yourself as the legitimate owner of the board,
so DO NOT LOSE IT!!! If you lose it, YOU WILL LOSE ACCESS TO YOUR BOARD!`)

	return nil
}

func CreatePublicKeyFromPrivate(actor *Actor, publicKeyPem string) error {
	publicFilename, err := GetActorPemFileFromDB(publicKeyPem)
	if err != nil {
		return util.MakeError(err, "CreatePublicKeyFromPrivate")
	}

	privateFilename := strings.ReplaceAll(publicFilename, "public.pem", "private.pem")
	if _, err := os.Stat(privateFilename); err == nil {
		// Not a lost cause
		priv, err := ioutil.ReadFile(privateFilename)
		if err != nil {
			return util.MakeError(err, "CreatePublicKeyFromPrivate")
		}

		block, _ := pem.Decode([]byte(priv))
		if block == nil || block.Type != "RSA PRIVATE KEY" {
			return errors.New("failed to decode PEM block containing public key")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return util.MakeError(err, "CreatePublicKeyFromPrivate")
		}

		publicKeyDer, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return util.MakeError(err, "CreatePublicKeyFromPrivate")
		}

		pubKeyBlock := pem.Block{
			Type:    "PUBLIC KEY",
			Headers: nil,
			Bytes:   publicKeyDer,
		}

		publicFileWriter, err := os.Create(publicFilename)
		if err != nil {
			return util.MakeError(err, "CreatePublicKeyFromPrivate")
		}

		if err := pem.Encode(publicFileWriter, &pubKeyBlock); err != nil {
			return util.MakeError(err, "CreatePublicKeyFromPrivate")
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

func GetActorPemFromDB(pemID string) (PublicKeyPem, error) {
	var pem PublicKeyPem

	query := `select id, owner, file from publickeypem where id=$1`

	if err := config.DB.QueryRow(query, pemID).Scan(&pem.Id, &pem.Owner, &pem.PublicKeyPem); err != nil {
		return pem, util.MakeError(err, "GetActorPemFromDB")
	}

	dir, _ := os.Getwd()
	dir = dir + "" + strings.Replace(pem.PublicKeyPem, ".", "", 1)
	f, err := os.ReadFile(dir)
	if err != nil {
		return pem, util.MakeError(err, "GetActorPemFromDB")
	}

	pem.PublicKeyPem = strings.ReplaceAll(string(f), "\r\n", `\n`)

	return pem, nil
}

func GetActorPemFileFromDB(pemID string) (string, error) {
	query := `select file from publickeypem where id=$1`
	rows, err := config.DB.Query(query, pemID)
	if err != nil {
		return "", util.MakeError(err, "GetActorPemFileFromDB")
	}

	defer rows.Close()

	var file string
	rows.Next()
	rows.Scan(&file)

	return file, nil
}

func StorePemToDB(actor Actor) error {
	query := "select publicKeyPem from actor where id=$1"
	rows, err := config.DB.Query(query, actor.Id)
	if err != nil {
		return util.MakeError(err, "StorePemToDB")
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
	if _, err := config.DB.Exec(query, publicKeyPem, actor.Id); err != nil {
		return util.MakeError(err, "StorePemToDB")
	}

	file := "./pem/board/" + actor.Name + "-public.pem"
	query = "insert into publicKeyPem (id, owner, file) values($1, $2, $3)"
	_, err = config.DB.Exec(query, publicKeyPem, actor.Id, file)
	return util.MakeError(err, "StorePemToDB")
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
