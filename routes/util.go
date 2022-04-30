package routes

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/gofiber/fiber/v2"
)

var ErrorPageLimit = errors.New("above page limit")

func getThemeCookie(c *fiber.Ctx) string {
	cookie := c.Cookies("theme")
	if cookie != "" {
		cookies := strings.SplitN(cookie, "=", 2)
		return cookies[0]
	}

	return "default"
}

func getPassword(r *fiber.Ctx) (string, string) {
	c := r.Cookies("session_token")

	sessionToken := c

	response, err := db.Cache.Do("GET", sessionToken)
	if err != nil {
		return "", ""
	}

	token := fmt.Sprintf("%s", response)

	parts := strings.Split(token, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}

func wantToServePage(actorName string, page int) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	// TODO: don't hard code?
	if page > 10 {
		return collection, serve, ErrorPageLimit
	}

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = activitypub.GetObjectFromDBPage(actor.Id, page)
		if err != nil {
			return collection, false, err
		}

		collection.Actor = &actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func wantToServeCatalog(actorName string) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = activitypub.GetObjectFromDBCatalog(actor.Id)
		if err != nil {
			return collection, false, err
		}

		collection.Actor = &actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func wantToServeArchive(actorName string) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = activitypub.GetActorCollectionDBType(actor.Id, "Archive")
		if err != nil {
			return collection, false, err
		}

		collection.Actor = &actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func hasValidation(ctx *fiber.Ctx, actor activitypub.Actor) bool {
	id, _ := getPassword(ctx)

	if id == "" || (id != actor.Id && id != config.Domain) {
		//http.Redirect(w, r, "/", http.StatusSeeOther)
		return false
	}

	return true
}
