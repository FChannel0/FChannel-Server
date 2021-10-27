package routes

import "github.com/gofiber/fiber/v2"

func Inbox(c *fiber.Ctx) error {
	// STUB

	return c.SendString("main inbox")
}
