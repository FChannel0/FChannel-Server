package routes

import "github.com/gofiber/fiber/v2"

func Outbox(c *fiber.Ctx) error {
	// STUB

	return c.SendString("main outbox")
}
