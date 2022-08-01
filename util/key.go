package util

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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

		config.CookieKey = encryptcookie.GenerateKey()
		log.Println("Generated new Cookie Key: ", config.CookieKey)
		if file, err = os.OpenFile(config.ConfigFile, os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			log.Println(fmt.Sprintf("Failed to write key to %s", config.ConfigFile))
			log.Println("If you are running in Docker, define it in COOKIE_KEY environment variable")
			log.Fatalln(err)
			return "", MakeError(err, "GetCookieKey")
		}

		defer file.Close()

		_, err = file.WriteString("\ncookie_key: " + config.CookieKey)
		if err != nil {
			log.Println(fmt.Sprintf("Failed to write key to %s", config.ConfigFile))
			log.Println("If you are running in Docker, define it in COOKIE_KEY environment variable")
			return "", MakeError(err, "GetCookieKey")
		}
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
