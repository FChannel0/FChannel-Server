package routes

import "github.com/gofiber/fiber/v2"

func AdminVerify(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin verify")
}

func AdminAuth(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin auth")
}

func AdminIndex(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin index")
}

func AdminAddBoard(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin add board")
}

func AdminPostNews(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin post news")
}

func AdminNewsDelete(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin news delete")
}
