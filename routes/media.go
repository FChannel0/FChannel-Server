package routes

import "github.com/gofiber/fiber/v2"

func ApiMedia(c *fiber.Ctx) error {
	return c.SendString("api media")
}
