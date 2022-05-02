package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofrs/uuid"
)

func AdminVerify(ctx *fiber.Ctx) error {
	identifier := ctx.FormValue("id")
	code := ctx.FormValue("code")

	var verify db.Verify
	verify.Identifier = identifier
	verify.Code = code

	j, _ := json.Marshal(&verify)

	req, err := http.NewRequest("POST", config.Domain+"/auth", bytes.NewBuffer(j))

	if err != nil {
		log.Println("error making verify req")
		return err
	}

	req.Header.Set("Content-Type", config.ActivityStreams)

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Println("error getting verify resp")
		return err
	}

	defer resp.Body.Close()

	rBody, _ := ioutil.ReadAll(resp.Body)

	body := string(rBody)

	if resp.StatusCode != 200 {
		return ctx.Redirect("/"+config.Key, http.StatusPermanentRedirect)
	}

	//TODO remove redis dependency
	sessionToken, _ := uuid.NewV4()

	_, err = db.Cache.Do("SETEX", sessionToken, "86400", body+"|"+verify.Code)
	if err != nil {
		return ctx.Redirect("/"+config.Key, http.StatusPermanentRedirect)
	}

	ctx.Cookie(&fiber.Cookie{
		Name:    "session_token",
		Value:   sessionToken.String(),
		Expires: time.Now().UTC().Add(60 * 60 * 48 * time.Second),
	})

	return ctx.Redirect("/", http.StatusSeeOther)
}

// TODO remove this route it is mostly unneeded
func AdminAuth(ctx *fiber.Ctx) error {
	var verify db.Verify

	err := json.Unmarshal(ctx.Body(), &verify)

	if err != nil {
		log.Println("error get verify from json")
		return err
	}

	v, _ := db.GetVerificationByCode(verify.Code)

	if v.Identifier == verify.Identifier {
		_, err := ctx.Write([]byte(v.Board))
		return err
	}

	ctx.Response().Header.SetStatusCode(http.StatusBadRequest)
	_, err = ctx.Write([]byte(""))

	return err
}

func AdminIndex(ctx *fiber.Ctx) error {
	fmt.Println("admin index")
	id, _ := db.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	if id == "" || (id != actor.Id && id != config.Domain) {
		return ctx.Render("verify", fiber.Map{})
	}

	actor, err := webfinger.GetActor(config.Domain)

	if err != nil {
		return err
	}

	follow, _ := webfinger.GetActorCollection(actor.Following)
	follower, _ := webfinger.GetActorCollection(actor.Followers)

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	var adminData AdminPage
	adminData.Following = following
	adminData.Followers = followers
	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Board.ModCred, _ = db.GetPasswordFromSession(ctx)
	adminData.Title = actor.Name + " Admin page"

	adminData.Boards = webfinger.Boards

	adminData.Board.Post.Actor = actor.Id

	adminData.PostBlacklist, _ = util.GetRegexBlacklistDB()

	adminData.Themes = &config.Themes

	return ctx.Render("admin", fiber.Map{
		"page": adminData,
	})
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
