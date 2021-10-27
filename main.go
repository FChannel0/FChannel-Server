package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/FChannel0/FChannel-Server/routes"
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
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Port = ":" + GetConfigValue("instanceport", "3000")
var TP = GetConfigValue("instancetp", "")
var Instance = GetConfigValue("instance", "")
var Domain = TP + "" + Instance
var TorInstance = IsOnion(Instance)

var authReq = []string{"captcha", "email", "passphrase"}

var supportedFiles = []string{"image/gif", "image/jpeg", "image/png", "image/webp", "image/apng", "video/mp4", "video/ogg", "video/webm", "audio/mpeg", "audio/ogg", "audio/wav", "audio/wave", "audio/x-wav"}

var SiteEmail = GetConfigValue("emailaddress", "") //contact@fchan.xyz
var SiteEmailPassword = GetConfigValue("emailpass", "")
var SiteEmailServer = GetConfigValue("emailserver", "") //mail.fchan.xyz
var SiteEmailPort = GetConfigValue("emailport", "")     //587

var TorProxy = GetConfigValue("torproxy", "") //127.0.0.1:9050

var PublicIndexing = strings.ToLower(GetConfigValue("publicindex", "false"))

var Salt = GetConfigValue("instancesalt", "")

var activitystreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""

var MediaHashs = make(map[string]string)

var ActorCache = make(map[string]Actor)

var Themes []string

var DB *sql.DB

func main() {

	CreatedNeededDirectories()

	InitCache()

	DB = ConnectDB()

	defer DB.Close()

	RunDatabaseSchema(DB)

	go MakeCaptchas(DB, 100)

	*Key = CreateKey(32)

	FollowingBoards = GetActorFollowingDB(DB, Domain)

	go StartupArchive(DB)

	go CheckInactive(DB)

	Boards = GetBoardCollection(DB)

	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if GetConfigValue("instancename", "") != "" {
		CreateNewBoardDB(DB, *CreateNewActor("", GetConfigValue("instancename", ""), GetConfigValue("instancesummary", ""), authReq, false))
		if PublicIndexing == "true" {
			AddInstanceToIndex(Domain)
		}
	}

	// get list of themes
	themes, err := ioutil.ReadDir("./static/css/themes")
	if err != nil {
		panic(err)
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			Themes = append(Themes, strings.TrimSuffix(f.Name(), e))
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

	app.Get("/"+*Key+"/", routes.AdminIndex)

	app.Get("/"+*Key+"/addboard", routes.AdminAddBoard)

	app.Get("/"+*Key+"/postnews", routes.AdminPostNews)
	app.Get("/"+*Key+"/newsdelete", routes.AdminNewsDelete)
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

		if !IsActorLocal(DB, TP+""+actorDomain[1]+""+actorDomain[0]) {
			c.Status(fiber.StatusBadRequest)
			return c.Send([]byte("actor not local"))
		}

		var finger Webfinger
		var link WebfingerLink

		finger.Subject = "acct:" + actorDomain[0] + "@" + actorDomain[1]
		link.Rel = "self"
		link.Type = "application/activity+json"
		link.Href = TP + "" + actorDomain[1] + "" + actorDomain[0]

		finger.Links = append(finger.Links, link)

		enc, _ := json.Marshal(finger)

		c.Set("Content-Type", activitystreams)
		return c.Send(enc)
	})

	app.Get("/api/media", func(c *fiber.Ctx) error {
		return c.SendString("api media")
	})

	fmt.Println("Server for " + Domain + " running on port " + Port)

	fmt.Println("Mod key: " + *Key)
	PrintAdminAuth(DB)

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

func ConnectDB() *sql.DB {

	host := GetConfigValue("dbhost", "localhost")
	port, _ := strconv.Atoi(GetConfigValue("dbport", "5432"))
	user := GetConfigValue("dbuser", "postgres")
	password := GetConfigValue("dbpass", "password")
	dbname := GetConfigValue("dbname", "server")

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)
	CheckError(err, "error with db connection")

	err = db.Ping()

	CheckError(err, "error with db ping")

	fmt.Println("Successfully connected DB")
	return db
}

func CreateKey(len int) string {
	var key string
	str := (CreateTripCode(RandomID(len)))
	for i := 0; i < len; i++ {
		key += fmt.Sprintf("%c", str[i])
	}
	return key
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

func CreateTripCode(input string) string {
	cmd := exec.Command("sha512sum")
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	CheckError(err, "error with create trip code")

	code := strings.Split(out.String(), " ")

	return code[0]
}

func GetActorFromPath(db *sql.DB, location string, prefix string) Actor {
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

	var nActor Actor

	nActor = GetActorByNameFromDB(db, actor)

	if nActor.Id == "" {
		nActor = GetActorByName(db, actor)
	}

	return nActor
}

func GetContentType(location string) string {
	elements := strings.Split(location, ";")
	if len(elements) > 0 {
		return elements[0]
	} else {
		return location
	}
}

func RandomID(size int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	domain := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rng := size
	newID := ""
	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}

	return newID
}

func CreateUniqueID(db *sql.DB, actor string) string {
	var newID string
	isUnique := false
	for !isUnique {
		newID = RandomID(8)

		query := fmt.Sprintf("select id from activitystream where id='%s/%s/%s'", Domain, actor, newID)

		rows, err := db.Query(query)

		CheckError(err, "error with unique id query")

		defer rows.Close()

		var count int = 0
		for rows.Next() {
			count += 1
		}

		if count < 1 {
			isUnique = true
		}
	}

	return newID
}

func CreateNewActor(board string, prefName string, summary string, authReq []string, restricted bool) *Actor {
	actor := new(Actor)

	var path string
	if board == "" {
		path = Domain
		actor.Name = "main"
	} else {
		path = Domain + "/" + board
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
	actor := GetActorFromDB(db, id)
	enc, _ := json.MarshalIndent(actor, "", "\t")
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetActorPost(w http.ResponseWriter, db *sql.DB, path string) {
	collection := GetCollectionFromPath(db, Domain+""+path)
	if len(collection.OrderedItems) > 0 {
		enc, _ := json.MarshalIndent(collection, "", "\t")
		w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		w.Write(enc)
	}
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().UTC()
	nObj.Updated = time.Now().UTC()

	return nObj
}

func AddFollowersToActivity(db *sql.DB, activity Activity) Activity {

	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		aFollowers := GetActorCollection(e + "/followers")
		for _, k := range aFollowers.Items {
			activity.To = append(activity.To, k.Id)
		}
	}

	var nActivity Activity

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

func CreateActivity(activityType string, obj ObjectBase) Activity {
	var newActivity Activity
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

func ProcessActivity(db *sql.DB, activity Activity) {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if GetActorFromDB(db, e).Id != "" {
				fmt.Println("actor is in the database")
			} else {
				fmt.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {

	} else if activityType == "Delete" {

	}
}

func CreatePreviewObject(obj ObjectBase) *NestedObjectBase {

	re := regexp.MustCompile(`/.+$`)

	mimetype := re.ReplaceAllString(obj.MediaType, "")

	var nPreview NestedObjectBase

	if mimetype != "image" {
		return &nPreview
	}

	re = regexp.MustCompile(`.+/`)

	file := re.ReplaceAllString(obj.MediaType, "")

	href := GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)

	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+href)

	err := cmd.Run()

	if CheckError(err, "error with resize attachment preview") != nil {
		var preview NestedObjectBase
		return &preview
	}

	return &nPreview
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]ObjectBase, *os.File) {
	contentType, _ := GetFileContentType(file)
	filename := header.Filename
	size := header.Size

	re := regexp.MustCompile(`.+/`)

	fileType := re.ReplaceAllString(contentType, "")

	tempFile, _ := ioutil.TempFile("./public", "*."+fileType)

	var nAttachment []ObjectBase
	var image ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = Domain + "/" + tempFile.Name()
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile
}

func ParseCommentForReplies(db *sql.DB, comment string, op string) []ObjectBase {

	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)
		str = TP + "" + str
		_, isReply := IsReplyToOP(db, op, str)
		if !IsInStringArray(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []ObjectBase
	for i := 0; i < len(links); i++ {
		_, isValid := CheckValidActivity(links[i])
		if isValid {
			var reply = new(ObjectBase)
			reply.Id = links[i]
			reply.Published = time.Now().UTC()
			validLinks = append(validLinks, *reply)
		}
	}

	return validLinks
}

func CheckValidActivity(id string) (Collection, bool) {
	var respCollection Collection

	re := regexp.MustCompile(`.+\.onion(.+)?`)
	if re.MatchString(id) {
		id = strings.Replace(id, "https", "http", 1)
	}

	req, err := http.NewRequest("GET", id, nil)

	if err != nil {
		fmt.Println("error with request")
	}

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

	if err != nil {
		fmt.Println("error with response")
		return respCollection, false
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &respCollection)

	if err != nil {
		panic(err)
	}

	if respCollection.AtContext.Context == "https://www.w3.org/ns/activitystreams" && respCollection.OrderedItems[0].Id != "" {
		return respCollection, true
	}

	return respCollection, false
}

func GetActor(id string) Actor {

	var respActor Actor

	if id == "" {
		return respActor
	}

	actor, instance := GetActorInstance(id)

	if ActorCache[actor+"@"+instance].Id != "" {
		respActor = ActorCache[actor+"@"+instance]
	} else {
		req, err := http.NewRequest("GET", strings.TrimSpace(id), nil)

		CheckError(err, "error with getting actor req")

		req.Header.Set("Accept", activitystreams)

		resp, err := RouteProxy(req)

		if err != nil {
			return respActor
		}

		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		err = json.Unmarshal(body, &respActor)

		if err != nil {
			return respActor
		}

		ActorCache[actor+"@"+instance] = respActor
	}

	return respActor
}

func GetActorCollection(collection string) Collection {
	var nCollection Collection

	if collection == "" {
		return nCollection
	}

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req "+collection)

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

	if err != nil {
		fmt.Println("error with getting actor collection resp " + collection)
		return nCollection
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			err = json.Unmarshal(body, &nCollection)

			CheckError(err, "error getting actor collection from body "+collection)
		}
	}

	return nCollection
}

func IsValidActor(id string) (Actor, bool) {

	actor := FingerActor(id)

	if actor.Id != "" {
		return actor, true
	}

	return actor, false
}

func IsActivityLocal(db *sql.DB, activity Activity) bool {
	for _, e := range activity.To {
		if GetActorFromDB(db, e).Id != "" {
			return true
		}
	}

	for _, e := range activity.Cc {
		if GetActorFromDB(db, e).Id != "" {
			return true
		}
	}

	if activity.Actor != nil && GetActorFromDB(db, activity.Actor.Id).Id != "" {
		return true
	}

	return false
}

func IsIDLocal(db *sql.DB, id string) bool {
	activity := GetActivityFromDB(db, id)
	return len(activity.OrderedItems) > 0
}

func IsActorLocal(db *sql.DB, id string) bool {
	actor := GetActorFromDB(db, id)

	if actor.Id != "" {
		return true
	}

	return false
}

func IsObjectLocal(db *sql.DB, id string) bool {

	query := `select id from activitystream where id=$1`

	rows, _ := db.Query(query, id)

	var nID string
	defer rows.Close()
	rows.Next()
	rows.Scan(&nID)

	if nID == "" {
		return false
	}

	return true
}

func IsObjectCached(db *sql.DB, id string) bool {

	query := `select id from cacheactivitystream where id=$1`
	rows, _ := db.Query(query, id)

	var nID string
	defer rows.Close()
	rows.Next()
	rows.Scan(&nID)

	if nID == "" {
		return false
	}

	return true
}

func GetObjectFromActivity(activity Activity) ObjectBase {
	return *activity.Object
}

func MakeCaptchas(db *sql.DB, total int) {
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

func IsReplyInThread(db *sql.DB, inReplyTo string, id string) bool {
	obj, _ := CheckValidActivity(inReplyTo)

	for _, e := range obj.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true
		}
	}
	return false
}

func SupportedMIMEType(mime string) bool {
	for _, e := range supportedFiles {
		if e == mime {
			return true
		}
	}

	return false
}

func DeleteReportActivity(db *sql.DB, id string) bool {

	query := `delete from reported where id=$1`

	_, err := db.Exec(query, id)

	if err != nil {
		CheckError(err, "error closing reported activity")
		return false
	}

	return true
}

func ReportActivity(db *sql.DB, id string, reason string) bool {

	if !IsIDLocal(db, id) {
		return false
	}

	actor := GetActivityFromDB(db, id)

	query := `select count from reported where id=$1`

	rows, err := db.Query(query, id)

	CheckError(err, "could not select count from reported")

	defer rows.Close()
	var count int
	for rows.Next() {
		rows.Scan(&count)
	}

	if count < 1 {
		query = `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		_, err := db.Exec(query, id, 1, actor.Actor.Id, reason)

		if err != nil {
			CheckError(err, "error inserting new reported activity")
			return false
		}

	} else {
		count = count + 1
		query = `update reported set count=$1 where id=$2`

		_, err := db.Exec(query, count, id)

		if err != nil {
			CheckError(err, "error updating reported activity")
			return false
		}
	}

	return true
}

func GetActorReported(w http.ResponseWriter, r *http.Request, db *sql.DB, id string) {

	auth := r.Header.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}

	if !HasAuth(db, verification[1], id) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}

	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems = GetActorReportedTotal(db, id)
	following.Items = GetActorReportedDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}

func MakeActivityRequestOutbox(db *sql.DB, activity Activity) {
	j, _ := json.Marshal(activity)

	if activity.Actor.Outbox == "" {
		return
	}

	req, err := http.NewRequest("POST", activity.Actor.Outbox, bytes.NewBuffer(j))

	CheckError(err, "error with sending activity req to outbox")

	re := regexp.MustCompile("https?://(www.)?")

	var instance string
	if activity.Actor.Id == Domain {
		instance = re.ReplaceAllString(Domain, "")
	} else {
		_, instance = GetActorInstance(activity.Actor.Id)
	}

	date := time.Now().UTC().Format(time.RFC1123)
	path := strings.Replace(activity.Actor.Outbox, instance, "", 1)

	path = re.ReplaceAllString(path, "")

	sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
	encSig, err := ActivitySign(db, *activity.Actor, sig)
	CheckError(err, "unable to sign activity response")
	signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

	req.Header.Set("Content-Type", activitystreams)
	req.Header.Set("Date", date)
	req.Header.Set("Signature", signature)
	req.Host = instance

	_, err = RouteProxy(req)

	CheckError(err, "error with sending activity resp to")
}

func MakeActivityRequest(db *sql.DB, activity Activity) {

	j, _ := json.MarshalIndent(activity, "", "\t")

	for _, e := range activity.To {
		if e != activity.Actor.Id {

			actor := FingerActor(e)

			if actor.Id != "" {
				_, instance := GetActorInstance(actor.Id)

				if actor.Inbox != "" {

					req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))

					CheckError(err, "error with sending activity req to")

					date := time.Now().UTC().Format(time.RFC1123)
					path := strings.Replace(actor.Inbox, instance, "", 1)

					re := regexp.MustCompile("https?://(www.)?")
					path = re.ReplaceAllString(path, "")

					sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
					encSig, err := ActivitySign(db, *activity.Actor, sig)
					CheckError(err, "unable to sign activity response")
					signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

					req.Header.Set("Content-Type", activitystreams)
					req.Header.Set("Date", date)
					req.Header.Set("Signature", signature)
					req.Host = instance

					_, err = RouteProxy(req)

					if err != nil {
						fmt.Println("error with sending activity resp to actor " + instance)
					}
				}
			}
		}
	}
}

func GetCollectionFromID(id string) Collection {
	var nColl Collection

	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "could not get collection from id req")

	req.Header.Set("Accept", activitystreams)

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

func GetConfigValue(value string, ifnone string) string {
	file, err := os.Open("config")

	CheckError(err, "there was an error opening the config file")

	defer file.Close()

	lines := bufio.NewScanner(file)

	for lines.Scan() {
		line := strings.SplitN(lines.Text(), ":", 2)
		if line[0] == value {
			return line[1]
		}
	}

	return ifnone
}

func PrintAdminAuth(db *sql.DB) {
	query := fmt.Sprintf("select identifier, code from boardaccess where board='%s' and type='admin'", Domain)

	rows, err := db.Query(query)

	CheckError(err, "Error getting Domain auth")

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
	id := RandomID(8)
	file := "/public/" + id + "." + _type

	for true {
		if _, err := os.Stat("." + file); err == nil {
			id = RandomID(8)
			file = "/public/" + id + "." + _type
		} else {
			return "/public/" + id + "." + _type
		}
	}

	return ""
}

func DeleteObjectRequest(db *sql.DB, id string) {
	var nObj ObjectBase
	var nActor Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectFromPath(db, id)

	actor := FingerActor(obj.Actor)
	activity.Actor = &actor

	followers := GetActorFollowDB(db, obj.Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(db, obj.Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(db, activity)
}

func DeleteObjectAndRepliesRequest(db *sql.DB, id string) {
	var nObj ObjectBase
	var nActor Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectByIDFromDB(db, id)

	activity.Actor.Id = obj.OrderedItems[0].Actor

	activity.Object = &obj.OrderedItems[0]

	followers := GetActorFollowDB(db, obj.OrderedItems[0].Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(db, obj.OrderedItems[0].Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(db, activity)
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

			var nPreview NestedObjectBase

			re = regexp.MustCompile(`/\w+$`)
			actor := re.ReplaceAllString(id, "")

			nPreview.Type = "Preview"
			nPreview.Id = fmt.Sprintf("%s/%s", actor, CreateUniqueID(db, actor))
			nPreview.Name = name
			nPreview.Href = Domain + "" + nHref
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
					WritePreviewToDB(db, nPreview)
					UpdateObjectWithPreview(db, id, nPreview.Id)
				}
			}
		}
	}
}

func UpdateObjectWithPreview(db *sql.DB, id string, preview string) {
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

func GetActorByName(db *sql.DB, name string) Actor {
	var actor Actor
	for _, e := range Boards {
		if e.Actor.Name == name {
			actor = e.Actor
		}
	}

	return actor
}

func GetActorCollectionReq(r *http.Request, collection string) Collection {
	var nCollection Collection

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req "+collection)

	_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", activitystreams)

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

func shortURL(actorName string, url string) string {

	re := regexp.MustCompile(`.+\/`)

	actor := re.FindString(actorName)

	urlParts := strings.Split(url, "|")

	op := urlParts[0]

	var reply string

	if len(urlParts) > 1 {
		reply = urlParts[1]
	}

	re = regexp.MustCompile(`\w+$`)
	temp := re.ReplaceAllString(op, "")

	if temp == actor {
		id := localShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}

		return id
	} else {
		id := remoteShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}

		return id
	}
}

func localShort(url string) string {
	re := regexp.MustCompile(`\w+$`)
	return re.FindString(StripTransferProtocol(url))
}

func remoteShort(url string) string {
	re := regexp.MustCompile(`\w+$`)

	id := re.FindString(StripTransferProtocol(url))

	re = regexp.MustCompile(`.+/.+/`)

	actorurl := re.FindString(StripTransferProtocol(url))

	re = regexp.MustCompile(`/.+/`)

	actorname := re.FindString(actorurl)

	actorname = strings.Replace(actorname, "/", "", -1)

	return "f" + actorname + "-" + id
}

func RouteProxy(req *http.Request) (*http.Response, error) {

	var proxyType = GetPathProxyType(req.URL.Host)

	if proxyType == "tor" {
		proxyUrl, err := url.Parse("socks5://" + TorProxy)

		CheckError(err, "error parsing tor proxy url")

		proxyTransport := &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		client := &http.Client{Transport: proxyTransport, Timeout: time.Second * 15}
		return client.Do(req)
	}

	return http.DefaultClient.Do(req)
}

func GetPathProxyType(path string) string {
	if TorProxy != "" {
		re := regexp.MustCompile(`(http://|http://)?(www.)?\w+\.onion`)
		onion := re.MatchString(path)
		if onion {
			return "tor"
		}
	}

	return "clearnet"
}

func RunDatabaseSchema(db *sql.DB) {
	query, err := ioutil.ReadFile("databaseschema.psql")
	CheckError(err, "could not read databaseschema.psql file")
	if _, err := db.Exec(string(query)); err != nil {
		CheckError(err, "could not exec databaseschema.psql")
	}
}

func CreatedNeededDirectories() {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		os.Mkdir("./public", 0755)
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		os.MkdirAll("./pem/board", 0700)
	}
}

//looks for actor with pattern of board@instance
func FingerActor(path string) Actor {

	var nActor Actor

	actor, instance := GetActorInstance(path)

	if actor == "" && instance == "" {
		return nActor
	}

	if ActorCache[actor+"@"+instance].Id != "" {
		nActor = ActorCache[actor+"@"+instance]
	} else {
		r := FingerRequest(actor, instance)
		if r != nil && r.StatusCode == 200 {
			defer r.Body.Close()

			body, _ := ioutil.ReadAll(r.Body)

			err := json.Unmarshal(body, &nActor)

			CheckError(err, "error getting fingerrequet resp from json body")

			ActorCache[actor+"@"+instance] = nActor
		}
	}

	return nActor
}

func FingerRequest(actor string, instance string) *http.Response {
	acct := "acct:" + actor + "@" + instance
	req, err := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)

	CheckError(err, "could not get finger request from id req")

	resp, err := RouteProxy(req)

	var finger Webfinger

	if err != nil {
		return resp
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		err := json.Unmarshal(body, &finger)

		CheckError(err, "error getting fingerrequet resp from json body")
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, err := http.NewRequest("GET", e.Href, nil)

				CheckError(err, "could not get finger request from id req")

				req.Header.Set("Accept", activitystreams)

				resp, err := RouteProxy(req)
				return resp
			}
		}
	}

	return resp
}

func GetActorInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if atFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)(www)?([\w\d-_.:]+)(/|\s+|\r|\r\n)?$`)
	mainActor := re.MatchString(path)
	if mainActor {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return "main", match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if httpFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 3 {
			if match[4] == "users" {
				return match[6], match[3]
			}

			return match[4], match[3]
		}
	}

	return "", ""
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

func AddInstanceToIndexDB(db *sql.DB, actor string) {

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

func GetCollectionFromReq(path string) Collection {
	req, err := http.NewRequest("GET", path, nil)
	CheckError(err, "error with getting collection from req")

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

	CheckError(err, "error getting resp from collection req")

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respCollection Collection

	_ = json.Unmarshal(body, &respCollection)

	return respCollection
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

func IsPostBlacklist(db *sql.DB, comment string) bool {
	postblacklist := GetRegexBlacklistDB(db)

	for _, e := range postblacklist {
		re := regexp.MustCompile(e.Regex)

		if re.MatchString(comment) {
			return true
		}
	}

	return false
}

func HasValidation(w http.ResponseWriter, r *http.Request, actor Actor) bool {
	id, _ := GetPasswordFromSession(r)

	if id == "" || (id != actor.Id && id != Domain) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return false
	}

	return true
}

func IsReplyToOP(db *sql.DB, op string, link string) (string, bool) {

	if op == link {
		return link, true
	}

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(link)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		link = re.ReplaceAllString(link, "")
		link = "%" + match[1] + "/" + link
	}

	query := `select id from replies where id like $1 and inreplyto=$2`

	rows, err := db.Query(query, link, op)

	CheckError(err, "error selecting in reply to op from db")

	var id string
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {

		return id, true
	}

	return "", false
}

func GetReplyOP(db *sql.DB, link string) string {

	query := `select id from replies where id in (select inreplyto from replies where id=$1) and inreplyto=''`

	rows, err := db.Query(query, link)

	CheckError(err, "could not get reply OP from db ")

	var id string

	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	return id
}

func StartupArchive(db *sql.DB) {
	for _, e := range FollowingBoards {
		ArchivePosts(db, GetActorFromDB(db, e.Id))
	}
}

func CheckInactive(db *sql.DB) {
	for true {
		CheckInactiveInstances(db)
		time.Sleep(24 * time.Hour)
	}
}

func CheckInactiveInstances(db *sql.DB) map[string]string {
	instances := make(map[string]string)
	query := `select following from following`
	rows, err := db.Query(query)

	CheckError(err, "cold not select instances from following")

	defer rows.Close()
	for rows.Next() {
		var instance string
		rows.Scan(&instance)
		instances[instance] = instance
	}

	query = `select follower from follower`
	rows, err = db.Query(query)

	CheckError(err, "cold not select instances from follower")

	defer rows.Close()
	for rows.Next() {
		var instance string
		rows.Scan(&instance)
		instances[instance] = instance
	}

	re := regexp.MustCompile(Domain + `(.+)?`)
	for _, e := range instances {
		actor := GetActor(e)
		if actor.Id == "" && !re.MatchString(e) {
			AddInstanceToInactiveDB(db, e)
		} else {
			DeleteInstanceFromInactiveDB(db, e)
		}
	}

	return instances
}

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc(
		"mod", mod,
	)

	engine.AddFunc(
		"sub", sub,
	)

	engine.AddFunc(
		"unixtoreadable", unixToReadable,
	)

	engine.AddFunc("proxy", func(url string) string {
		return MediaProxy(url)
	})

	engine.AddFunc("short", func(actorName string, url string) string {
		return shortURL(actorName, url)
	})

	engine.AddFunc("parseAttachment", func(obj ObjectBase, catalog bool) template.HTML {
		return ParseAttachment(obj, catalog)
	})

	engine.AddFunc("parseContent", func(board Actor, op string, content string, thread ObjectBase) template.HTML {
		return ParseContent(DB, board, op, content, thread)
	})

	engine.AddFunc("shortImg", func(url string) string {
		return ShortImg(url)
	})

	engine.AddFunc("convertSize", func(size int64) string {
		return ConvertSize(size)
	})

	engine.AddFunc("isOnion", func(url string) bool {
		return IsOnion(url)
	})

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor := FingerActor(actorId)
		title := strings.ReplaceAll(ParseLinkTitle(actor.Id, op, content), `/\&lt;`, ">")
		link := "<a href=\"" + actor.Name + "/" + shortURL(actor.Outbox, op) + "#" + shortURL(actor.Outbox, id) + "\" title=\"" + title + "\" class=\"replyLink\">&gt;&gt;" + shortURL(actor.Outbox, id) + "</a>"
		return template.HTML(link)
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc(
		"timeToReadableLong", timeToReadableLong,
	)

	engine.AddFunc(
		"timeToUnix", timeToUnix,
	)

	engine.AddFunc(
		"sub", sub,
	)
}
