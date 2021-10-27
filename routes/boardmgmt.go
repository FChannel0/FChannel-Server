package routes

import "github.com/gofiber/fiber/v2"

func BoardBanMedia(c *fiber.Ctx) error {
	return c.SendString("board ban media")
}

func BoardDelete(c *fiber.Ctx) error {
	return c.SendString("board delete")
}

func BoardDeleteAttach(c *fiber.Ctx) error {
	return c.SendString("board delete attach")
}

func BoardMarkSensitive(c *fiber.Ctx) error {
	return c.SendString("board mark sensitive")
}

func BoardRemove(c *fiber.Ctx) error {
	return c.SendString("board remove")
}

func BoardRemoveAttach(c *fiber.Ctx) error {
	return c.SendString("board remove attach")
}

func BoardAddToIndex(c *fiber.Ctx) error {
	return c.SendString("board add to index")
}

func BoardPopArchive(c *fiber.Ctx) error {
	return c.SendString("board pop archive")
}

func BoardAutoSubscribe(c *fiber.Ctx) error {
	return c.SendString("board auto subscribe")
}

func BoardBlacklist(c *fiber.Ctx) error {
	return c.SendString("board blacklist")
}

func BoardReport(c *fiber.Ctx) error {
	return c.SendString("board report")
}
