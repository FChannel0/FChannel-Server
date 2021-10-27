package routes

import "github.com/gofiber/fiber/v2"

func Index(c *fiber.Ctx) error {
	return c.SendString("index")
}
