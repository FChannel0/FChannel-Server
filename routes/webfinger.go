package routes

import (
	"encoding/json"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func Webfinger(c *fiber.Ctx) error {
	acct := c.Query("resource")

	if len(acct) < 1 {
		c.Status(fiber.StatusBadRequest)
		return c.Send([]byte("resource needs a value"))
	}

	acct = strings.Replace(acct, "acct:", "", -1)

	actorDomain := strings.Split(acct, "@")

	if len(actorDomain) < 2 {
		c.Status(fiber.StatusBadRequest)
		return c.Send([]byte("accpets only subject form of acct:board@instance"))
	}

	if actorDomain[0] == "main" {
		actorDomain[0] = ""
	} else {
		actorDomain[0] = "/" + actorDomain[0]
	}

	actor := activitypub.Actor{Id: config.TP + "" + actorDomain[1] + "" + actorDomain[0]}
	if res, err := actor.IsLocal(); err == nil && !res {
		c.Status(fiber.StatusBadRequest)
		return c.Send([]byte("actor not local"))
	} else if err != nil {
		return util.MakeError(err, "Webfinger")
	}

	var finger activitypub.Webfinger
	var link activitypub.WebfingerLink

	finger.Subject = "acct:" + actorDomain[0] + "@" + actorDomain[1]
	link.Rel = "self"
	link.Type = "application/activity+json"
	link.Href = config.TP + "" + actorDomain[1] + "" + actorDomain[0]

	finger.Links = append(finger.Links, link)

	enc, _ := json.Marshal(finger)

	c.Set("Content-Type", config.ActivityStreams)
	return c.Send(enc)
}
