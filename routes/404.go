package routes

import (
	"github.com/gofiber/fiber/v2"
)

func NotFound(c *fiber.Ctx) error {
	return c.Status(404).Render("404", fiber.Map{}, "layouts/main")
}
