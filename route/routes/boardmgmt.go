package routes

import "github.com/gofiber/fiber/v2"

func BoardBanMedia(ctx *fiber.Ctx) error {
	return ctx.SendString("board ban media")
}

func BoardDelete(ctx *fiber.Ctx) error {
	return ctx.SendString("board delete")
}

func BoardDeleteAttach(ctx *fiber.Ctx) error {
	return ctx.SendString("board delete attach")
}

func BoardMarkSensitive(ctx *fiber.Ctx) error {
	return ctx.SendString("board mark sensitive")
}

func BoardRemove(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove")
}

func BoardRemoveAttach(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove attach")
}

func BoardAddToIndex(ctx *fiber.Ctx) error {
	return ctx.SendString("board add to index")
}

func BoardPopArchive(ctx *fiber.Ctx) error {
	return ctx.SendString("board pop archive")
}

func BoardAutoSubscribe(ctx *fiber.Ctx) error {
	return ctx.SendString("board auto subscribe")
}

func BoardBlacklist(ctx *fiber.Ctx) error {
	return ctx.SendString("board blacklist")
}

func BoardReport(ctx *fiber.Ctx) error {
	return ctx.SendString("board report")
}
