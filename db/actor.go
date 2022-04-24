package db

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/gofiber/fiber/v2"
)

func GetActorFromPath(location string, prefix string) (activitypub.Actor, error) {
	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)

	var actor string

	if len(match) < 1 {
		actor = "/"
	} else {
		actor = strings.Replace(match[1], "/", "", -1)
	}

	if actor == "/" || actor == "outbox" || actor == "inbox" || actor == "following" || actor == "followers" {
		actor = "main"
	}

	var nActor activitypub.Actor

	nActor, err := GetActorByNameFromDB(actor)
	if err != nil {
		return nActor, err
	}

	if nActor.Id == "" {
		nActor = GetActorByName(actor)
	}

	return nActor, nil
}

func GetActorByName(name string) activitypub.Actor {
	var actor activitypub.Actor
	for _, e := range Boards {
		if e.Actor.Name == name {
			actor = e.Actor
		}
	}

	return actor
}

func GetActorInfo(ctx *fiber.Ctx, id string) error {
	actor, err := GetActorFromDB(id)
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err = ctx.Write(enc)

	return err
}
