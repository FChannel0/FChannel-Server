package routes

import "github.com/gofiber/fiber/v2"

func ActorIndex(c *fiber.Ctx) error {
	// STUB
	// TODO: OutboxGet, already implemented
	return c.SendString("actor index")
}

func ActorPostGet(c *fiber.Ctx) error {
	// STUB
	// TODO: PostGet
	return c.SendString("actor post get")
}

func ActorInbox(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor inbox")
}

func ActorOutbox(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor outbox")
}

func ActorFollowing(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor following")
}

func ActorFollowers(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor followers")
}

func ActorReported(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor reported")
}

func ActorArchive(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor archive")
}

func ActorPost(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor post")
}
