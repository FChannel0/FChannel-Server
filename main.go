package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/routes"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"

	// "github.com/gofrs/uuid"
	_ "github.com/lib/pq"

	"html/template"
	// "io"
	"io/ioutil"
	// "log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"
)

var authReq = []string{"captcha", "email", "passphrase"}

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

	db.FollowingBoards, err = db.GetActorFollowingDB(config.Domain)
	if err != nil {
		panic(err)
	}

	go db.StartupArchive()

	go db.CheckInactive()

	db.Boards, err = db.GetBoardCollection()
	if err != nil {
		panic(err)
	}

	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if config.InstanceName != "" {
		if _, err = db.CreateNewBoardDB(*CreateNewActor("", config.InstanceName, config.InstanceSummary, authReq, false)); err != nil {
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

	// Allow access to public media folder
	fileServer := http.FileServer(http.Dir("./public"))
	http.Handle("/public/", http.StripPrefix("/public", neuter(fileServer)))

	javascriptFiles := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static", neuter(javascriptFiles)))

	/* Routing and templates */

	template := html.New("./views", ".html")

	TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName: "FChannel",
		Views:   template,
	})

	/*
	 Main actor
	*/

	app.Get("/", routes.Index)

	app.Get("/inbox", routes.Inbox)
	app.Get("/outbox", routes.Outbox)

	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)

	/*
	 Board actor
	*/

	app.Get("/:actor", routes.OutboxGet)

	app.Get("/:actor/:post", routes.ActorPostGet)
	app.Get("/post", routes.ActorPost)

	app.Get("/:actor/inbox", routes.ActorInbox)
	app.Get("/:actor/outbox", routes.ActorOutbox)

	app.Get("/:actor/following", routes.ActorFollowing)
	app.Get("/:actor/followers", routes.ActorFollowers)

	app.Get("/:actor/reported", routes.ActorReported)
	app.Get("/:actor/archive", routes.ActorArchive)

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

		if res, err := db.IsActorLocal(config.TP + "" + actorDomain[1] + "" + actorDomain[0]); err == nil && !res {
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

	// 404 handler
	app.Use(routes.NotFound)

	app.Static("/public", "./public")
	app.Static("/static", "./views")

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

func GetContentType(location string) string {
	elements := strings.Split(location, ";")
	if len(elements) > 0 {
		return elements[0]
	} else {
		return location
	}
}

func CreateNewActor(board string, prefName string, summary string, authReq []string, restricted bool) *activitypub.Actor {
	actor := new(activitypub.Actor)

	var path string
	if board == "" {
		path = config.Domain
		actor.Name = "main"
	} else {
		path = config.Domain + "/" + board
		actor.Name = board
	}

	actor.Type = "Group"
	actor.Id = fmt.Sprintf("%s", path)
	actor.Following = fmt.Sprintf("%s/following", actor.Id)
	actor.Followers = fmt.Sprintf("%s/followers", actor.Id)
	actor.Inbox = fmt.Sprintf("%s/inbox", actor.Id)
	actor.Outbox = fmt.Sprintf("%s/outbox", actor.Id)
	actor.PreferredUsername = prefName
	actor.Restricted = restricted
	actor.Summary = summary
	actor.AuthRequirement = authReq

	return actor
}

func GetActorInfo(w http.ResponseWriter, id string) error {
	actor, err := db.GetActorFromDB(id)
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(actor, "", "\t")
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	_, err = w.Write(enc)
	return err
}

func GetActorPost(w http.ResponseWriter, path string) error {
	collection, err := db.GetCollectionFromPath(config.Domain + "" + path)
	if err != nil {
		return err
	}

	if len(collection.OrderedItems) > 0 {
		enc, err := json.MarshalIndent(collection, "", "\t")
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		_, err = w.Write(enc)
		return err
	}

	return nil
}

func CreateObject(objType string) activitypub.ObjectBase {
	var nObj activitypub.ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().UTC()
	nObj.Updated = time.Now().UTC()

	return nObj
}

func AddFollowersToActivity(activity activitypub.Activity) (activitypub.Activity, error) {
	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		aFollowers, err := webfinger.GetActorCollection(e + "/followers")
		if err != nil {
			return activity, err
		}

		for _, k := range aFollowers.Items {
			activity.To = append(activity.To, k.Id)
		}
	}

	var nActivity activitypub.Activity

	for _, e := range activity.To {
		var alreadyTo = false
		for _, k := range nActivity.To {
			if e == k || e == activity.Actor.Id {
				alreadyTo = true
			}
		}

		if !alreadyTo {
			nActivity.To = append(nActivity.To, e)
		}
	}

	activity.To = nActivity.To

	return activity, nil
}

func CreateActivity(activityType string, obj activitypub.ObjectBase) (activitypub.Activity, error) {
	var newActivity activitypub.Activity

	actor, err := webfinger.FingerActor(obj.Actor)
	if err != nil {
		return newActivity, err
	}

	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = &actor
	newActivity.Object = &obj

	for _, e := range obj.To {
		if obj.Actor != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor != e {
			newActivity.Cc = append(newActivity.Cc, e)
		}
	}

	return newActivity, nil
}

func ProcessActivity(activity activitypub.Activity) error {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if res, err := db.GetActorFromDB(e); err == nil && res.Id != "" {
				fmt.Println("actor is in the database")
			} else if err != nil {
				return err
			} else {
				fmt.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {
		// TODO: okay?
		return errors.New("not implemented")
	} else if activityType == "Delete" {
		return errors.New("not implemented")
	}

	return nil
}

func CreatePreviewObject(obj activitypub.ObjectBase) *activitypub.NestedObjectBase {
	re := regexp.MustCompile(`/.+$`)

	mimetype := re.ReplaceAllString(obj.MediaType, "")

	var nPreview activitypub.NestedObjectBase

	if mimetype != "image" {
		return &nPreview
	}

	re = regexp.MustCompile(`.+/`)

	file := re.ReplaceAllString(obj.MediaType, "")

	href := util.GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = config.Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)

	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+href)

	if err := cmd.Run(); err != nil {
		// TODO: previously we would call CheckError here
		var preview activitypub.NestedObjectBase
		return &preview
	}

	return &nPreview
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]activitypub.ObjectBase, *os.File, error) {
	contentType, err := GetFileContentType(file)
	if err != nil {
		return nil, nil, err
	}

	filename := header.Filename
	size := header.Size

	re := regexp.MustCompile(`.+/`)

	fileType := re.ReplaceAllString(contentType, "")

	tempFile, err := ioutil.TempFile("./public", "*."+fileType)
	if err != nil {
		return nil, nil, err
	}

	var nAttachment []activitypub.ObjectBase
	var image activitypub.ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = config.Domain + "/" + tempFile.Name()
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile, nil
}

func ParseCommentForReplies(comment string, op string) ([]activitypub.ObjectBase, error) {

	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)
		str = config.TP + "" + str
		_, isReply, err := db.IsReplyToOP(op, str)
		if err != nil {
			return nil, err
		}

		if !util.IsInStringArray(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []activitypub.ObjectBase
	for i := 0; i < len(links); i++ {
		_, isValid, err := webfinger.CheckValidActivity(links[i])
		if err != nil {
			return nil, err
		}

		if isValid {
			var reply activitypub.ObjectBase
			reply.Id = links[i]
			reply.Published = time.Now().UTC()
			validLinks = append(validLinks, reply)
		}
	}

	return validLinks, nil
}

func IsValidActor(id string) (activitypub.Actor, bool, error) {
	actor, err := webfinger.FingerActor(id)
	return actor, actor.Id != "", err
}

func IsActivityLocal(activity activitypub.Activity) (bool, error) {
	for _, e := range activity.To {
		if res, err := db.GetActorFromDB(e); err == nil && res.Id != "" {
			return true, nil
		} else if err != nil {
			return false, err
		}
	}

	for _, e := range activity.Cc {
		if res, err := db.GetActorFromDB(e); err == nil && res.Id != "" {
			return true, nil
		} else if err != nil {
			return false, err
		}
	}

	if res, err := db.GetActorFromDB(activity.Actor.Id); err == nil && activity.Actor != nil && res.Id != "" {
		return true, nil
	} else if err != nil {
		return false, err
	}

	return false, nil
}

func GetObjectFromActivity(activity activitypub.Activity) activitypub.ObjectBase {
	return *activity.Object
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

func GetFileContentType(out multipart.File) (string, error) {
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	out.Seek(0, 0)

	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func SupportedMIMEType(mime string) bool {
	for _, e := range config.SupportedFiles {
		if e == mime {
			return true
		}
	}

	return false
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
	following.TotalItems, err = db.GetActorReportedTotal(id)
	if err != nil {
		return err
	}

	following.Items, err = db.GetActorReportedDB(id)
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

func GetCollectionFromID(id string) (activitypub.Collection, error) {
	var nColl activitypub.Collection

	req, err := http.NewRequest("GET", id, nil)
	if err != nil {
		return nColl, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return nColl, err
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			if err := json.Unmarshal(body, &nColl); err != nil {
				return nColl, err
			}
		}
	}

	return nColl, nil
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

	activity, err := CreateActivity("Delete", nObj)
	if err != nil {
		return err
	}

	obj, err := db.GetObjectFromPath(id)
	if err != nil {
		return err
	}

	actor, err := webfinger.FingerActor(obj.Actor)
	if err != nil {
		return err
	}
	activity.Actor = &actor

	followers, err := db.GetActorFollowDB(obj.Actor)
	if err != nil {
		return err
	}

	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := db.GetActorFollowingDB(obj.Actor)
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

	activity, err := CreateActivity("Delete", nObj)
	if err != nil {
		return err
	}

	obj, err := db.GetObjectByIDFromDB(id)
	if err != nil {
		return err
	}

	activity.Actor.Id = obj.OrderedItems[0].Actor

	activity.Object = &obj.OrderedItems[0]

	followers, err := db.GetActorFollowDB(obj.OrderedItems[0].Actor)
	if err != nil {
		return err
	}
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := db.GetActorFollowingDB(obj.OrderedItems[0].Actor)
	if err != nil {
		return err
	}

	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	return db.MakeActivityRequest(activity)
}

func ResizeAttachmentToPreview() error {
	return db.GetObjectsWithoutPreviewsCallback(func(id, href, mediatype, name string, size int, published time.Time) error {
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
			uid, err := db.CreateUniqueID(actor)
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
					if err := db.WritePreviewToDB(nPreview); err != nil {
						return err
					}
					if err := db.UpdateObjectWithPreview(id, nPreview.Id); err != nil {
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

func ParseCommentForReply(comment string) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		links = append(links, str)
	}

	if len(links) > 0 {
		_, isValid, err := webfinger.CheckValidActivity(strings.ReplaceAll(links[0], ">", ""))
		if err != nil {
			return "", err
		}

		if isValid {
			return links[0], nil
		}
	}

	return "", nil
}

func GetActorCollectionReq(r *http.Request, collection string) (activitypub.Collection, error) {
	var nCollection activitypub.Collection

	req, err := http.NewRequest("GET", collection, nil)
	if err != nil {
		return nCollection, err
	}

	// TODO: rewrite this for fiber
	pass := "FIXME"
	//_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", config.ActivityStreams)

	req.Header.Set("Authorization", "Basic "+pass)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return nCollection, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		if err := json.Unmarshal(body, &nCollection); err != nil {
			return nCollection, err
		}
	}

	return nCollection, nil
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
	followers, err := GetCollectionFromID("https://fchan.xyz/followers")
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
	followers, err := GetCollectionFromID("https://fchan.xyz/followers")
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
		return db.AddFollower("https://fchan.xyz", nActor.Id)
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
