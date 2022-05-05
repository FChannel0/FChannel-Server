package routes

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
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

	ctx.Cookie(&fiber.Cookie{
		Name:    "session_token",
		Value:   body + "|" + verify.Code,
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

func AdminFollow(ctx *fiber.Ctx) error {
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	following := regexp.MustCompile(`(.+)\/following`)
	followers := regexp.MustCompile(`(.+)\/followers`)

	follow := ctx.FormValue("follow")
	actorId := ctx.FormValue("actor")

	//follow all of boards following
	if following.MatchString(follow) {
		followingActor, _ := webfinger.FingerActor(follow)
		col, _ := webfinger.GetActorCollection(followingActor.Following)

		var nObj activitypub.ObjectBase
		nObj.Id = followingActor.Id

		col.Items = append(col.Items, nObj)

		for _, e := range col.Items {
			if isFollowing, _ := activitypub.IsAlreadyFollowing(actorId, e.Id); !isFollowing && e.Id != config.Domain && e.Id != actorId {
				followActivity, _ := db.MakeFollowActivity(actorId, e.Id)

				if actor, _ := webfinger.FingerActor(e.Id); actor.Id != "" {
					db.MakeActivityRequestOutbox(followActivity)
				}
			}
		}

		//follow all of boards followers
	} else if followers.MatchString(follow) {
		followersActor, _ := webfinger.FingerActor(follow)
		col, _ := webfinger.GetActorCollection(followersActor.Followers)

		var nObj activitypub.ObjectBase
		nObj.Id = followersActor.Id

		col.Items = append(col.Items, nObj)

		for _, e := range col.Items {
			if isFollowing, _ := activitypub.IsAlreadyFollowing(actorId, e.Id); !isFollowing && e.Id != config.Domain && e.Id != actorId {
				followActivity, _ := db.MakeFollowActivity(actorId, e.Id)
				if actor, _ := webfinger.FingerActor(e.Id); actor.Id != "" {
					db.MakeActivityRequestOutbox(followActivity)
				}
			}
		}

		//do a normal follow to a single board
	} else {
		followActivity, _ := db.MakeFollowActivity(actorId, follow)

		if isLocal, _ := activitypub.IsActorLocal(followActivity.Object.Actor); !isLocal && followActivity.Actor.Id == config.Domain {
			_, err := ctx.Write([]byte("main board can only follow local boards. Create a new board and then follow outside boards from it."))
			return err
		}

		if actor, _ := webfinger.FingerActor(follow); actor.Id != "" {
			db.MakeActivityRequestOutbox(followActivity)
		}
	}

	var redirect string

	if actor.Name != "main" {
		redirect = "/" + actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminAddBoard(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)

	if hasValidation := db.HasValidation(ctx, actor); !hasValidation {
		return nil
	}

	var newActorActivity activitypub.Activity
	var board activitypub.Actor

	var restrict bool
	if ctx.FormValue("restricted") == "True" {
		restrict = true
	} else {
		restrict = false
	}

	board.Name = ctx.FormValue("name")
	board.PreferredUsername = ctx.FormValue("prefname")
	board.Summary = ctx.FormValue("summary")
	board.Restricted = restrict

	newActorActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActorActivity.Type = "New"

	var nobj activitypub.ObjectBase
	newActorActivity.Actor = &actor
	newActorActivity.Object = &nobj

	newActorActivity.Object.Alias = board.Name
	newActorActivity.Object.Name = board.PreferredUsername
	newActorActivity.Object.Summary = board.Summary
	newActorActivity.Object.Sensitive = board.Restricted

	db.MakeActivityRequestOutbox(newActorActivity)
	return ctx.Redirect("/"+config.Key, http.StatusSeeOther)
}

func AdminPostNews(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin post news")
}

func AdminNewsDelete(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin news delete")
}

func AdminActorIndex(ctx *fiber.Ctx) error {
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	follow, _ := webfinger.GetActorCollection(actor.Following)
	follower, _ := webfinger.GetActorCollection(actor.Followers)
	reported, _ := activitypub.GetActorCollectionReq(actor.Id + "/reported")

	var following []string
	var followers []string
	var reports []db.Report

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	for _, e := range reported.Items {
		var r db.Report
		r.Count = int(e.Size)
		r.ID = e.Id
		r.Reason = e.Content
		reports = append(reports, r)
	}

	localReports, _ := db.GetLocalReportDB(actor.Name)

	for _, e := range localReports {
		var r db.Report
		r.Count = e.Count
		r.ID = e.ID
		r.Reason = e.Reason
		reports = append(reports, r)
	}

	var data AdminPage
	data.Following = following
	data.Followers = followers
	data.Reported = reports
	data.Domain = config.Domain
	data.IsLocal, _ = activitypub.IsActorLocal(actor.Id)

	data.Title = "Manage /" + actor.Name + "/"
	data.Boards = webfinger.Boards
	data.Board.Name = actor.Name
	data.Board.Actor = actor
	data.Key = config.Key
	data.Board.TP = config.TP

	data.Board.Post.Actor = actor.Id

	data.AutoSubscribe, _ = activitypub.GetActorAutoSubscribeDB(actor.Id)

	data.Themes = &config.Themes

	data.RecentPosts = activitypub.GetRecentPostsDB(actor.Id)

	if cookie := ctx.Cookies("theme"); cookie != "" {
		data.ThemeCookie = cookie
	}

	return ctx.Render("manage", fiber.Map{
		"page": data,
	})
}
