package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/routes"
	"github.com/FChannel0/FChannel-Server/util"
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
			panic(err)
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

	app.Static("/public", "./public")
	app.Static("/static", "./views")

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

	app.Get("/:actor", routes.ActorIndex) //OutboxGet)

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

		if !IsActorLocal(config.TP + "" + actorDomain[1] + "" + actorDomain[0]) {
			c.Status(fiber.StatusBadRequest)
			return c.Send([]byte("actor not local"))
		}

		var finger Webfinger
		var link WebfingerLink

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
		return c.SendString("api media")
	})

	// 404 handler
	app.Use(routes.NotFound)

	fmt.Println("Mod key: " + config.Key)
	PrintAdminAuth()

	app.Listen(Port)
}

func CheckError(e error, m string) error {
	if e != nil {
		fmt.Println()
		fmt.Println(m)
		fmt.Println()
		panic(e)
	}

	return e
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

func GetActorInfo(w http.ResponseWriter, db *sql.DB, id string) {
	actor := GetActorFromDB(id)
	enc, _ := json.MarshalIndent(actor, "", "\t")
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetActorPost(w http.ResponseWriter, db *sql.DB, path string) {
	collection := GetCollectionFromPath(Domain + "" + path)
	if len(collection.OrderedItems) > 0 {
		enc, _ := json.MarshalIndent(collection, "", "\t")
		w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		w.Write(enc)
	}
}

func CreateObject(objType string) activitypub.ObjectBase {
	var nObj activitypub.ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().UTC()
	nObj.Updated = time.Now().UTC()

	return nObj
}

func AddFollowersToActivity(activity activitypub.Activity) activitypub.Activity {

	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		aFollowers := GetActorCollection(e + "/followers")
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

	return activity
}

func CreateActivity(activityType string, obj activitypub.ObjectBase) activitypub.Activity {
	var newActivity activitypub.Activity
	actor := FingerActor(obj.Actor)

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

	return newActivity
}

func ProcessActivity(activity activitypub.Activity) {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if GetActorFromDB(e).Id != "" {
				fmt.Println("actor is in the database")
			} else {
				fmt.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {

	} else if activityType == "Delete" {

	}
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

	href := GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = config.Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)

	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+href)

	err := cmd.Run()

	if CheckError(err, "error with resize attachment preview") != nil {
		var preview activitypub.NestedObjectBase
		return &preview
	}

	return &nPreview
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]activitypub.ObjectBase, *os.File) {
	contentType, _ := GetFileContentType(file)
	filename := header.Filename
	size := header.Size

	re := regexp.MustCompile(`.+/`)

	fileType := re.ReplaceAllString(contentType, "")

	tempFile, _ := ioutil.TempFile("./public", "*."+fileType)

	var nAttachment []activitypub.ObjectBase
	var image activitypub.ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = config.Domain + "/" + tempFile.Name()
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile
}

func ParseCommentForReplies(comment string, op string) []activitypub.ObjectBase {

	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)
		str = config.TP + "" + str
		_, isReply := IsReplyToOP(op, str)
		if !IsInStringArray(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []activitypub.ObjectBase
	for i := 0; i < len(links); i++ {
		_, isValid := CheckValidActivity(links[i])
		if isValid {
			var reply = new(activitypub.ObjectBase)
			reply.Id = links[i]
			reply.Published = time.Now().UTC()
			validLinks = append(validLinks, *reply)
		}
	}

	return validLinks
}

func IsValidActor(id string) (activitypub.Actor, bool) {

	actor := FingerActor(id)

	if actor.Id != "" {
		return actor, true
	}

	return actor, false
}

func IsActivityLocal(activity activitypub.Activity) bool {
	for _, e := range activity.To {
		if GetActorFromDB(e).Id != "" {
			return true
		}
	}

	for _, e := range activity.Cc {
		if GetActorFromDB(e).Id != "" {
			return true
		}
	}

	if activity.Actor != nil && GetActorFromDB(activity.Actor.Id).Id != "" {
		return true
	}

	return false
}

func GetObjectFromActivity(activity activitypub.Activity) activitypub.ObjectBase {
	return *activity.Object
}

func MakeCaptchas(total int) {
	difference := total - GetCaptchaTotal(db)

	for i := 0; i < difference; i++ {
		CreateNewCaptcha(db)
	}
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
	for _, e := range supportedFiles {
		if e == mime {
			return true
		}
	}

	return false
}

func GetActorReported(w http.ResponseWriter, r *http.Request, db *sql.DB, id string) {

	auth := r.Header.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}

	if !HasAuth(verification[1], id) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}

	var following activitypub.Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems = GetActorReportedTotal(id)
	following.Items = GetActorReportedDB(id)

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", config.ActivityStreams)
	w.Write(enc)
}

func GetCollectionFromID(id string) activitypub.Collection {
	var nColl activitypub.Collection

	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "could not get collection from id req")

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := RouteProxy(req)

	if err != nil {
		return nColl
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			err = json.Unmarshal(body, &nColl)

			CheckError(err, "error getting collection resp from json body")
		}
	}

	return nColl
}

func PrintAdminAuth(db *sql.DB) {
	query := fmt.Sprintf("select identifier, code from boardaccess where board='%s' and type='admin'", config.Domain)

	rows, err := db.Query(query)

	CheckError(err, "Error getting config.Domain auth")

	var code string
	var identifier string

	rows.Next()
	rows.Scan(&identifier, &code)

	fmt.Println("Admin Login: " + identifier + ", Code: " + code)
}

func IsInStringArray(array []string, value string) bool {
	for _, e := range array {
		if e == value {
			return true
		}
	}
	return false
}

func GetUniqueFilename(_type string) string {
	id := util.RandomID(8)
	file := "/public/" + id + "." + _type

	for true {
		if _, err := os.Stat("." + file); err == nil {
			id = util.RandomID(8)
			file = "/public/" + id + "." + _type
		} else {
			return "/public/" + id + "." + _type
		}
	}

	return ""
}

func DeleteObjectRequest(id string) {
	var nObj activitypub.ObjectBase
	var nActor activitypub.Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectFromPath(id)

	actor := FingerActor(obj.Actor)
	activity.Actor = &actor

	followers := GetActorFollowDB(obj.Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(obj.Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(activity)
}

func DeleteObjectAndRepliesRequest(id string) {
	var nObj activitypub.ObjectBase
	var nActor activitypub.Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectByIDFromDB(id)

	activity.Actor.Id = obj.OrderedItems[0].Actor

	activity.Object = &obj.OrderedItems[0]

	followers := GetActorFollowDB(obj.OrderedItems[0].Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(obj.OrderedItems[0].Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(activity)
}

func ResizeAttachmentToPreview(db *sql.DB) {
	query := `select id, href, mediatype, name, size, published from activitystream where id in (select attachment from activitystream where attachment!='' and preview='')`

	rows, err := db.Query(query)

	CheckError(err, "error getting attachments")

	defer rows.Close()
	for rows.Next() {

		var id string
		var href string
		var mediatype string
		var name string
		var size int
		var published time.Time

		rows.Scan(&id, &href, &mediatype, &name, &size, &published)

		re := regexp.MustCompile(`^\w+`)

		_type := re.FindString(mediatype)

		if _type == "image" {

			re = regexp.MustCompile(`.+/`)

			file := re.ReplaceAllString(mediatype, "")

			nHref := GetUniqueFilename(file)

			var nPreview activitypub.NestedObjectBase

			re = regexp.MustCompile(`/\w+$`)
			actor := re.ReplaceAllString(id, "")

			nPreview.Type = "Preview"
			nPreview.Id = fmt.Sprintf("%s/%s", actor, CreateUniqueID(actor))
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

				err := cmd.Run()

				CheckError(err, "error with resize attachment preview")

				if err == nil {
					fmt.Println(objFile + " -> " + nHref)
					WritePreviewToDB(nPreview)
					UpdateObjectWithPreview(id, nPreview.Id)
				}
			}
		}
	}
}

func UpdateObjectWithPreview(id string, preview string) {
	query := `update activitystream set preview=$1 where attachment=$2`

	_, err := db.Exec(query, preview, id)

	CheckError(err, "could not update activity stream with preview")

}

func ParseCommentForReply(comment string) string {

	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		links = append(links, str)
	}

	if len(links) > 0 {
		_, isValid := CheckValidActivity(strings.ReplaceAll(links[0], ">", ""))

		if isValid {
			return links[0]
		}
	}

	return ""
}

func GetActorCollectionReq(r *http.Request, collection string) activitypub.Collection {
	var nCollection activitypub.Collection

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req "+collection)

	_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", config.ActivityStreams)

	req.Header.Set("Authorization", "Basic "+pass)

	resp, err := RouteProxy(req)

	CheckError(err, "error with getting actor collection resp "+collection)

	if resp.StatusCode == 200 {

		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		err = json.Unmarshal(body, &nCollection)

		CheckError(err, "error getting actor collection from body "+collection)
	}

	return nCollection
}

func CreatedNeededDirectories() {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		os.Mkdir("./public", 0755)
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		os.MkdirAll("./pem/board", 0700)
	}
}

func AddInstanceToIndex(actor string) {
	// if local testing enviroment do not add to index
	re := regexp.MustCompile(`(.+)?(localhost|\d+\.\d+\.\d+\.\d+)(.+)?`)
	if re.MatchString(actor) {
		return
	}

	followers := GetCollectionFromID("https://fchan.xyz/followers")

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == actor {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		req, err := http.NewRequest("GET", "https://fchan.xyz/addtoindex?id="+actor, nil)

		CheckError(err, "error with add instance to actor index req")

		_, err = http.DefaultClient.Do(req)

		CheckError(err, "error with add instance to actor index resp")
	}
}

func AddInstanceToIndexDB(actor string) {

	//sleep to be sure the webserver is fully initialized
	//before making finger request
	time.Sleep(15 * time.Second)

	nActor := FingerActor(actor)

	if nActor.Id == "" {
		return
	}

	followers := GetCollectionFromID("https://fchan.xyz/followers")

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == nActor.Id {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		query := `insert into follower (id, follower) values ($1, $2)`

		_, err := db.Exec(query, "https://fchan.xyz", nActor.Id)

		CheckError(err, "Error with add to index query")
	}
}

func HashMedia(media string) string {
	h := sha256.New()
	h.Write([]byte(media))
	return hex.EncodeToString(h.Sum(nil))
}

func HashBytes(media []byte) string {
	h := sha256.New()
	h.Write(media)
	return hex.EncodeToString(h.Sum(nil))
}

func RouteImages(w http.ResponseWriter, media string) {

	req, err := http.NewRequest("GET", MediaHashs[media], nil)

	CheckError(err, "error with Route Images req")

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)

	if err != nil || resp.StatusCode != 200 {
		fileBytes, err := ioutil.ReadFile("./static/notfound.png")

		CheckError(err, "could not get /static/notfound.png file bytes")

		w.Write(fileBytes)
		return
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Set(name, value)
		}
	}

	w.Write(body)
}

func HasValidation(w http.ResponseWriter, r *http.Request, actor activitypub.Actor) bool {
	id, _ := GetPasswordFromSession(r)

	if id == "" || (id != actor.Id && id != config.Domain) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return false
	}

	return true
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
		actor := FingerActor(actorId)
		title := strings.ReplaceAll(ParseLinkTitle(actor.Id, op, content), `/\&lt;`, ">")
		link := fmt.Sprintf("<a href=\"%s/%s#%s\" title=\"%s\" class=\"replyLink\">&gt;&gt;%s</a>", actor.Name, util.ShortURL(actor.Outbox, op), util.ShortURL(actor.Outbox, id), title, util.ShortURL(actor.Outbox, id))
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt",
		func(post activitypub.ObjectBase) string {
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
