package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gofiber/fiber/v2"
)

func Following(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowingResp(ctx)
}

func Followers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowersResp(ctx)
}
