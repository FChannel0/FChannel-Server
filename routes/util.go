package routes

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func getThemeCookie(c *fiber.Ctx) string {
	cookie := c.Cookies("theme")
	if cookie != "" {
		cookies := strings.SplitN(cookie, "=", 2)
		return cookies[0]
	}

	return "default"
}

func getPassword(r *fiber.Ctx) (string, string) {
	c := r.Cookies("session_token")

	sessionToken := c

	response, err := cache.Do("GET", sessionToken)
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
