package routes

import "github.com/gofiber/fiber/v2"

func Following(c *fiber.Ctx) error {
	// STUB

	return c.SendString("main following")
}

func Followers(c *fiber.Ctx) error {
	// STUB

	return c.SendString("main followers")
}
