package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2"
)

func Following(ctx *fiber.Ctx) error {
	return activitypub.GetActorFollowing(ctx, config.Domain)
}

func Followers(ctx *fiber.Ctx) error {
	// STUB
	return activitypub.GetActorFollowers(ctx, config.Domain)
}
