package routes

import "github.com/gofiber/fiber/v2"

func NewsGet(c *fiber.Ctx) error {
	return c.SendString("news get")
}
