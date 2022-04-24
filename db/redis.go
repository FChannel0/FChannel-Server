package db

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gomodule/redigo/redis"
)

var Cache redis.Conn

func InitCache() error {
	conn, err := redis.DialURL(config.Redis)
	Cache = conn
	return err
}

func CloseCache() error {
	return Cache.Close()
}

func GetClientKey() (string, error) {
	file, err := os.Open("clientkey")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var line string
	for scanner.Scan() {
		line = fmt.Sprintf("%s", scanner.Text())
	}

	return line, nil
}

func GetPasswordFromSession(c *fiber.Ctx) (string, string) {

	cookie := c.Cookies("session_token")

	if cookie == "" {
		return "", ""
	}

	sessionToken := cookie

	response, err := Cache.Do("GET", sessionToken)

	if err != nil {
		return "", ""
	}

	token := fmt.Sprintf("%s", response)

	parts := strings.Split(token, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}

/* TODO: Convert to fiber ctx
func CheckSession(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	c, err := r.Cookie("session_token")

	if err != nil {
		if err == http.ErrNoCookie {
			w.WriteHeader(http.StatusUnauthorized)
			return nil, err
		}

		w.WriteHeader(http.StatusBadRequest)
		return nil, err
	}

	sessionToken := c.Value

	response, err := Cache.Do("GET", sessionToken)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return nil, err
	}
	if response == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, err
	}

	return response, nil
	}
*/
