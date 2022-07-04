package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/route"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func AdminVerify(ctx *fiber.Ctx) error {
	identifier := ctx.FormValue("id")
	code := ctx.FormValue("code")

	var verify util.Verify
	verify.Identifier = identifier
	verify.Code = code

	j, _ := json.Marshal(&verify)

	req, err := http.NewRequest("POST", config.Domain+"/"+config.Key+"/auth", bytes.NewBuffer(j))

	if err != nil {
		return util.MakeError(err, "AdminVerify")
	}

	req.Header.Set("Content-Type", config.ActivityStreams)

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return util.MakeError(err, "AdminVerify")
	}

	defer resp.Body.Close()

	rBody, _ := ioutil.ReadAll(resp.Body)

	body := string(rBody)

	if resp.StatusCode != 200 {
		return ctx.Redirect("/", http.StatusPermanentRedirect)
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
	var verify util.Verify

	err := json.Unmarshal(ctx.Body(), &verify)

	if err != nil {
		return util.MakeError(err, "AdminAuth")
	}

	v, _ := util.GetVerificationByCode(verify.Code)

	if v.Identifier == verify.Identifier {
		_, err := ctx.Write([]byte(v.Board))
		return util.MakeError(err, "AdminAuth")
	}

	ctx.Response().Header.SetStatusCode(http.StatusBadRequest)
	_, err = ctx.Write([]byte(""))

	return util.MakeError(err, "AdminAuth")
}

func AdminIndex(ctx *fiber.Ctx) error {
	id, _ := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	if id == "" || (id != actor.Id && id != config.Domain) {
		return ctx.Render("verify", fiber.Map{"key": config.Key})
	}

	actor, err := activitypub.GetActor(config.Domain)

	if err != nil {
		return util.MakeError(err, "AdminIndex")
	}

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()
	follower, _ := reqActivity.GetCollection()

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	var adminData route.AdminPage
	adminData.Following = following
	adminData.Followers = followers

	var reported = make(map[string][]db.Reports)

	for _, e := range following {
		re := regexp.MustCompile(`.*/(.+)$`)
		boards := re.FindStringSubmatch(e)
		reports, _ := db.GetLocalReport(boards[1])

		for _, k := range reports {
			reported[k.Actor.Name] = append(reported[k.Actor.Name], k)
		}
	}

	for k, e := range reported {
		sort.Sort(db.ReportsSortDesc(e))
		reported[k] = e
	}

	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	adminData.Title = actor.Name + " Admin page"

	adminData.Boards = webfinger.Boards

	adminData.Board.Post.Actor = actor.Id

	adminData.Instance, _ = activitypub.GetActorFromDB(config.Domain)

	adminData.PostBlacklist, _ = util.GetRegexBlacklist()

	adminData.Meta.Description = adminData.Title
	adminData.Meta.Url = adminData.Board.Actor.Id
	adminData.Meta.Title = adminData.Title

	adminData.Themes = &config.Themes

	return ctx.Render("admin", fiber.Map{
		"page":    adminData,
		"reports": reported,
	}, "layouts/main")
}

func AdminFollow(ctx *fiber.Ctx) error {
	follow := ctx.FormValue("follow")
	actorId := ctx.FormValue("actor")

	actor := activitypub.Actor{Id: actorId}
	followActivity, _ := actor.MakeFollowActivity(follow)

	objActor := activitypub.Actor{Id: followActivity.Object.Actor}

	if isLocal, _ := objActor.IsLocal(); !isLocal && followActivity.Actor.Id == config.Domain {
		_, err := ctx.Write([]byte("main board can only follow local boards. Create a new board and then follow outside boards from it."))
		return util.MakeError(err, "AdminIndex")
	}

	if actor, _ := activitypub.FingerActor(follow); actor.Id != "" {
		if err := followActivity.MakeRequestOutbox(); err != nil {
			return util.MakeError(err, "AdminFollow")
		}
	}

	var redirect string
	actor, _ = webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Name != "main" {
		redirect = actor.Name
	}

	time.Sleep(time.Duration(500) * time.Millisecond)

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminAddBoard(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)

	if hasValidation := actor.HasValidation(ctx); !hasValidation {
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
	newActorActivity.Object = nobj

	newActorActivity.Object.Alias = board.Name
	newActorActivity.Object.Name = board.PreferredUsername
	newActorActivity.Object.Summary = board.Summary
	newActorActivity.Object.Sensitive = board.Restricted

	newActorActivity.MakeRequestOutbox()

	time.Sleep(time.Duration(500) * time.Millisecond)

	return ctx.Redirect("/"+config.Key, http.StatusSeeOther)
}

func AdminActorIndex(ctx *fiber.Ctx) error {
	var data route.AdminPage

	id, pass := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	var hasAuth bool
	hasAuth, data.Board.ModCred = util.HasAuth(pass, actor.Id)

	if !hasAuth || (id != actor.Id && id != config.Domain) {
		return ctx.Render("verify", fiber.Map{"key": config.Key})
	}

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()

	reqActivity.Id = actor.Followers
	follower, _ := reqActivity.GetCollection()

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	data.Following = following
	data.Followers = followers

	reports, _ := db.GetLocalReport(actor.Name)

	var reported = make(map[string][]db.Reports)
	for _, k := range reports {
		reported[k.Actor.Name] = append(reported[k.Actor.Name], k)
	}

	for k, e := range reported {
		sort.Sort(db.ReportsSortDesc(e))
		reported[k] = e
	}

	data.Domain = config.Domain
	data.IsLocal, _ = actor.IsLocal()
	data.Title = "Manage /" + actor.Name + "/"
	data.Boards = webfinger.Boards
	data.Board.Name = actor.Name
	data.Board.Actor = actor
	data.Key = config.Key
	data.Board.TP = config.TP

	data.Board.Post.Actor = actor.Id

	data.Instance, _ = activitypub.GetActorFromDB(config.Domain)

	data.AutoSubscribe, _ = actor.GetAutoSubscribe()

	jannies, err := actor.GetJanitors()

	if err != nil {
		return util.MakeError(err, "AdminActorIndex")
	}

	data.Meta.Description = data.Title
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes

	data.RecentPosts, _ = actor.GetRecentPosts()

	if cookie := ctx.Cookies("theme"); cookie != "" {
		data.ThemeCookie = cookie
	}

	return ctx.Render("manage", fiber.Map{
		"page":    data,
		"jannies": jannies,
		"reports": reported,
	}, "layouts/main")
}

func AdminAddJanny(ctx *fiber.Ctx) error {
	id, pass := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	hasAuth, _type := util.HasAuth(pass, actor.Id)

	if !hasAuth || _type != "admin" || (id != actor.Id && id != config.Domain) {
		return util.MakeError(errors.New("Error"), "AdminJanny")
	}

	var verify util.Verify
	verify.Type = "janitor"
	verify.Identifier = actor.Id
	verify.Label = ctx.FormValue("label")

	if err := actor.CreateVerification(verify); err != nil {
		return util.MakeError(err, "CreateNewBoardDB")
	}

	var redirect string
	actor, _ = webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminEditSummary(ctx *fiber.Ctx) error {
	id, pass := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	hasAuth, _type := util.HasAuth(pass, actor.Id)

	if !hasAuth || _type != "admin" || (id != actor.Id && id != config.Domain) {
		return util.MakeError(errors.New("Error"), "AdminEditSummary")
	}

	summary := ctx.FormValue("summary")

	query := `update actor set summary=$1 where id=$2`
	if _, err := config.DB.Exec(query, summary, actor.Id); err != nil {
		return util.MakeError(err, "AdminEditSummary")
	}

	var redirect string
	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)

}

func AdminDeleteJanny(ctx *fiber.Ctx) error {
	id, pass := util.GetPasswordFromSession(ctx)
	actor, _ := webfinger.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	hasAuth, _type := util.HasAuth(pass, actor.Id)

	if !hasAuth || _type != "admin" || (id != actor.Id && id != config.Domain) {
		return util.MakeError(errors.New("Error"), "AdminJanny")
	}

	var verify util.Verify
	verify.Code = ctx.Query("code")

	if err := actor.DeleteVerification(verify); err != nil {
		return util.MakeError(err, "AdminDeleteJanny")
	}

	var redirect string

	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}
