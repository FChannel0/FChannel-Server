package db

import (
	"os"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
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
