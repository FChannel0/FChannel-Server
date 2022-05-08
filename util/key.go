package util

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"math/rand"
	"os"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2/middleware/encryptcookie"
)

const domain = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func CreateKey(len int) (string, error) {
	// TODO: provided that CreateTripCode still uses sha512, the max len can be 128 at most.
	if len > 128 {
		return "", MakeError(errors.New("len is greater than 128"), "CreateKey")
	}

	str := CreateTripCode(RandomID(len))
	return str[:len], nil
}

func CreateTripCode(input string) string {
	out := sha512.Sum512([]byte(input))

	return hex.EncodeToString(out[:])
}

func GetCookieKey() (string, error) {
	if config.CookieKey == "" {
		var file *os.File
		var err error

		if file, err = os.OpenFile("config/config-init", os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			return "", MakeError(err, "GetCookieKey")
		}

		defer file.Close()

		config.CookieKey = encryptcookie.GenerateKey()
		file.WriteString("cookiekey:" + config.CookieKey)
	}

	return config.CookieKey, nil
}

func RandomID(size int) string {
	rng := size
	newID := strings.Builder{}

	for i := 0; i < rng; i++ {
		newID.WriteByte(domain[rand.Intn(len(domain))])
	}

	return newID.String()
}
