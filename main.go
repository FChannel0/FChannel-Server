package main

import (
	"encoding/json"
	"fmt"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/routes"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html"

	// "github.com/gofrs/uuid"
	_ "github.com/lib/pq"

	"html/template"
	// "io"
	"io/ioutil"
	// "log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"
)

var MediaHashs = make(map[string]string)

var Themes []string

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var err error

	CreatedNeededDirectories()

	db.ConnectDB()
	defer db.Close()

	db.InitCache()
	defer db.CloseCache()

	db.RunDatabaseSchema()

	go MakeCaptchas(100)

	config.Key = util.CreateKey(32)

	webfinger.FollowingBoards, err = activitypub.GetActorFollowingDB(config.Domain)

	if err != nil {
		panic(err)
	}

	go db.StartupArchive()

	go db.CheckInactive()

	webfinger.Boards, err = webfinger.GetBoardCollection()

	if err != nil {
		panic(err)
	}

	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if config.InstanceName != "" {
		if _, err = db.CreateNewBoardDB(*activitypub.CreateNewActor("", config.InstanceName, config.InstanceSummary, config.AuthReq, false)); err != nil {
			//panic(err)
		}

		if config.PublicIndexing == "true" {
			// TODO: comment out later
			//AddInstanceToIndex(config.Domain)
		}
	}

	// get list of themes
	themes, err := ioutil.ReadDir("./static/css/themes")
	if err != nil {
		panic(err)
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			config.Themes = append(config.Themes, strings.TrimSuffix(f.Name(), e))
		}
	}

	/* Routing and templates */

	template := html.New("./views", ".html")
	template.Debug(true)

	TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName: "FChannel",
		Views:   template,
	})

	app.Use(logger.New())

	app.Static("/static", "./views")
	app.Static("/public", "./public")

	/*
	 Main actor
	*/

	app.Get("/", routes.Index)

	app.Get("/inbox", routes.Inbox)
	app.Get("/outbox", routes.Outbox)

	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)

	/*
	 Admin routes
	*/

	app.Get("/verify", routes.AdminVerify)

	app.Get("/auth", routes.AdminAuth)

	app.Get("/"+config.Key+"/", routes.AdminIndex)

	app.Get("/"+config.Key+"/addboard", routes.AdminAddBoard)

	app.Get("/"+config.Key+"/postnews", routes.AdminPostNews)
	app.Get("/"+config.Key+"/newsdelete", routes.AdminNewsDelete)
	app.Get("/news", routes.NewsGet)

	/*
	 Board managment
	*/

	app.Get("/banmedia", routes.BoardBanMedia)
	app.Get("/delete", routes.BoardDelete)

	app.Get("/deleteattach", routes.BoardDeleteAttach)
	app.Get("/marksensitive", routes.BoardMarkSensitive)

	app.Get("/remove", routes.BoardRemove)
	app.Get("/removeattach", routes.BoardRemoveAttach)

	app.Get("/addtoindex", routes.BoardAddToIndex)

	app.Get("/poparchive", routes.BoardPopArchive)

	app.Get("/autosubscribe", routes.BoardAutoSubscribe)

	app.Get("/blacklist", routes.BoardBlacklist)
	app.Get("/report", routes.BoardBlacklist)

	app.Get("/.well-known/webfinger", func(c *fiber.Ctx) error {
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

		if res, err := activitypub.IsActorLocal(config.TP + "" + actorDomain[1] + "" + actorDomain[0]); err == nil && !res {
			c.Status(fiber.StatusBadRequest)
			return c.Send([]byte("actor not local"))
		} else if err != nil {
			return err
		}

		var finger webfinger.Webfinger
		var link webfinger.WebfingerLink

		finger.Subject = "acct:" + actorDomain[0] + "@" + actorDomain[1]
		link.Rel = "self"
		link.Type = "application/activity+json"
		link.Href = config.TP + "" + actorDomain[1] + "" + actorDomain[0]

		finger.Links = append(finger.Links, link)

		enc, _ := json.Marshal(finger)

		c.Set("Content-Type", config.ActivityStreams)
		return c.Send(enc)
	})

	app.Get("/api/media", func(c *fiber.Ctx) error {
		if c.Query("hash") != "" {
			return RouteImages(c, c.Query("hash"))
		}

		return c.SendStatus(404)
	})

	/*
	 Board actor
	*/

	app.Get("/:actor", routes.OutboxGet)
	app.Post("/:actor", routes.ActorPost)

	app.Get("/:actor/catalog", routes.CatalogGet)
	app.Get("/:actor/:post", routes.PostGet)

	app.Get("/:actor/inbox", routes.ActorInbox)
	app.Post("/:actor/outbox", routes.ActorOutbox)

	app.Get("/:actor/following", routes.ActorFollowing)
	app.Get("/:actor/followers", routes.ActorFollowers)

	app.Get("/:actor/reported", routes.ActorReported)
	app.Get("/:actor/archive", routes.ActorArchive)

	//404 handler
	app.Use(routes.NotFound)

	fmt.Println("Mod key: " + config.Key)
	PrintAdminAuth()

	app.Listen(config.Port)
}

func neuter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func IsValidActor(id string) (activitypub.Actor, bool, error) {
	actor, err := webfinger.FingerActor(id)
	return actor, actor.Id != "", err
}

func MakeCaptchas(total int) error {
	dbtotal, err := db.GetCaptchaTotal()
	if err != nil {
		return err
	}

	difference := total - dbtotal

	for i := 0; i < difference; i++ {
		if err := db.CreateNewCaptcha(); err != nil {
			return err
		}
	}

	return nil
}

func GetActorReported(w http.ResponseWriter, r *http.Request, id string) error {
	auth := r.Header.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(""))
		return err
	}

	if res, err := db.HasAuth(verification[1], id); err == nil && !res {
		w.WriteHeader(http.StatusBadRequest)
		_, err = w.Write([]byte(""))
		return err
	} else if err != nil {
		return err
	}

	var following activitypub.Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = activitypub.GetActorReportedTotal(id)
	if err != nil {
		return err
	}

	following.Items, err = activitypub.GetActorReportedDB(id)
	if err != nil {
		return err
	}

	enc, err := json.MarshalIndent(following, "", "\t")
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", config.ActivityStreams)

	_, err = w.Write(enc)
	return err
}

func PrintAdminAuth() error {
	identifier, code, err := db.GetAdminAuth()
	if err != nil {
		return err
	}

	fmt.Println("Admin Login: " + identifier + ", Code: " + code)
	return nil
}

func DeleteObjectRequest(id string) error {
	var nObj activitypub.ObjectBase
	var nActor activitypub.Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity, err := webfinger.CreateActivity("Delete", nObj)
	if err != nil {
		return err
	}

	obj, err := activitypub.GetObjectFromPath(id)
	if err != nil {
		return err
	}

	actor, err := webfinger.FingerActor(obj.Actor)
	if err != nil {
		return err
	}
	activity.Actor = &actor

	followers, err := activitypub.GetActorFollowDB(obj.Actor)
	if err != nil {
		return err
	}

	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := activitypub.GetActorFollowingDB(obj.Actor)
	if err != nil {
		return err
	}
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	return db.MakeActivityRequest(activity)
}

func DeleteObjectAndRepliesRequest(id string) error {
	var nObj activitypub.ObjectBase
	var nActor activitypub.Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity, err := webfinger.CreateActivity("Delete", nObj)
	if err != nil {
		return err
	}

	obj, err := activitypub.GetObjectByIDFromDB(id)
	if err != nil {
		return err
	}

	activity.Actor.Id = obj.OrderedItems[0].Actor

	activity.Object = &obj.OrderedItems[0]

	followers, err := activitypub.GetActorFollowDB(obj.OrderedItems[0].Actor)
	if err != nil {
		return err
	}
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := activitypub.GetActorFollowingDB(obj.OrderedItems[0].Actor)
	if err != nil {
		return err
	}

	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	return db.MakeActivityRequest(activity)
}

func ResizeAttachmentToPreview() error {
	return activitypub.GetObjectsWithoutPreviewsCallback(func(id, href, mediatype, name string, size int, published time.Time) error {
		re := regexp.MustCompile(`^\w+`)

		_type := re.FindString(mediatype)

		if _type == "image" {

			re = regexp.MustCompile(`.+/`)

			file := re.ReplaceAllString(mediatype, "")

			nHref := util.GetUniqueFilename(file)

			var nPreview activitypub.NestedObjectBase

			re = regexp.MustCompile(`/\w+$`)
			actor := re.ReplaceAllString(id, "")

			nPreview.Type = "Preview"
			uid, err := util.CreateUniqueID(actor)
			if err != nil {
				return err
			}

			nPreview.Id = fmt.Sprintf("%s/%s", actor, uid)
			nPreview.Name = name
			nPreview.Href = config.Domain + "" + nHref
			nPreview.MediaType = mediatype
			nPreview.Size = int64(size)
			nPreview.Published = published
			nPreview.Updated = published

			re = regexp.MustCompile(`/public/.+`)

			objFile := re.FindString(href)

			if id != "" {
				cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+nHref)

				if err := cmd.Run(); err == nil {
					fmt.Println(objFile + " -> " + nHref)
					if err := activitypub.WritePreviewToDB(nPreview); err != nil {
						return err
					}
					if err := activitypub.UpdateObjectWithPreview(id, nPreview.Id); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}

		return nil
	})
}

func CreatedNeededDirectories() {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		os.Mkdir("./public", 0755)
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		os.MkdirAll("./pem/board", 0700)
	}
}

func AddInstanceToIndex(actor string) error {
	// TODO: completely disabling this until it is actually reasonable to turn it on
	// only actually allow this when it more or less works, i.e. can post, make threads, manage boards, etc
	return nil

	// if local testing enviroment do not add to index
	re := regexp.MustCompile(`(.+)?(localhost|\d+\.\d+\.\d+\.\d+)(.+)?`)
	if re.MatchString(actor) {
		return nil
	}

	// also while i'm here
	// TODO: maybe allow different indexes?
	followers, err := activitypub.GetCollectionFromID("https://fchan.xyz/followers")
	if err != nil {
		return err
	}

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == actor {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		req, err := http.NewRequest("GET", "https://fchan.xyz/addtoindex?id="+actor, nil)
		if err != nil {
			return err
		}

		if _, err := http.DefaultClient.Do(req); err != nil {
			return err
		}
	}

	return nil
}

func AddInstanceToIndexDB(actor string) error {
	// TODO: completely disabling this until it is actually reasonable to turn it on
	// only actually allow this when it more or less works, i.e. can post, make threads, manage boards, etc
	return nil

	//sleep to be sure the webserver is fully initialized
	//before making finger request
	time.Sleep(15 * time.Second)

	nActor, err := webfinger.FingerActor(actor)
	if err != nil {
		return err
	}

	if nActor.Id == "" {
		return nil
	}

	// TODO: maybe allow different indexes?
	followers, err := activitypub.GetCollectionFromID("https://fchan.xyz/followers")
	if err != nil {
		return err
	}

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == nActor.Id {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		return activitypub.AddFollower("https://fchan.xyz", nActor.Id)
	}

	return nil
}

func RouteImages(ctx *fiber.Ctx, media string) error {
	req, err := http.NewRequest("GET", MediaHashs[media], nil)
	if err != nil {
		return err
	}

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fileBytes, err := ioutil.ReadFile("./static/notfound.png")
		if err != nil {
			return err
		}

		return ctx.Send(fileBytes)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	for name, values := range resp.Header {
		for _, value := range values {
			ctx.Append(name, value)
		}
	}

	return ctx.Send(body)
}

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc("mod", func(i, j int) bool {
		return i%j == 0
	})

	engine.AddFunc("sub", func(i, j int) int {
		return i - j
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc("unixtoreadable", func(u int) string {
		return time.Unix(int64(u), 0).Format("Jan 02, 2006")
	})

	engine.AddFunc("timeToReadableLong", func(t time.Time) string {
		return t.Format("01/02/06(Mon)15:04:05")
	})

	engine.AddFunc("timeToUnix", func(t time.Time) string {
		return fmt.Sprint(t.Unix())
	})

	engine.AddFunc("proxy", MediaProxy)

	// previously short
	engine.AddFunc("shortURL", util.ShortURL)

	engine.AddFunc("parseAttachment", ParseAttachment)

	engine.AddFunc("parseContent", ParseContent)

	engine.AddFunc("shortImg", util.ShortImg)

	engine.AddFunc("convertSize", util.ConvertSize)

	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor, err := webfinger.FingerActor(actorId)
		if err != nil {
			// TODO: figure out what to do here
			panic(err)
		}

		title := strings.ReplaceAll(ParseLinkTitle(actor.Id, op, content), `/\&lt;`, ">")
		link := fmt.Sprintf("<a href=\"%s/%s#%s\" title=\"%s\" class=\"replyLink\">&gt;&gt;%s</a>", actor.Name, util.ShortURL(actor.Outbox, op), util.ShortURL(actor.Outbox, id), title, util.ShortURL(actor.Outbox, id))
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt", func(post activitypub.ObjectBase) string {
		var returnString string

		if post.Name != "" {
			returnString = post.Name + "| " + post.Content
		} else {
			returnString = post.Content
		}

		re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

		match := re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = match[0] + "..."
		}

		re = regexp.MustCompile(`(^.+\|)`)

		match = re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = strings.Replace(returnString, match[0], "<b>"+match[0]+"</b>", 1)
			returnString = strings.Replace(returnString, "|", ":", 1)
		}

		return returnString
	})
}
