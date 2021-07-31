package main

import (
	"net/http"
	"html/template"
	"database/sql"
	_ "github.com/lib/pq"
	"strings"
	"strconv"
	"sort"
	"regexp"
	"time"
	"fmt"
)

var Key *string = new(string)

var FollowingBoards []ObjectBase

var Boards []Board

type Board struct{
	Name string
	Actor Actor
	Summary string
	PrefName string
	InReplyTo string
	Location string
	To string
	RedirectTo string
	Captcha string
	CaptchaCode string
	ModCred string
	Domain string
	TP string
	Restricted bool
	Post ObjectBase
}

type PageData struct {
	Title string
	PreferredUsername string
	Board Board
	Pages []int
	CurrentPage int
	TotalPage int
	Boards []Board
	Posts []ObjectBase
	Key string
	PostId string
	Instance Actor
	InstanceIndex []ObjectBase
	ReturnTo string
	NewsItems []NewsItem
	BoardRemainer []int
}

type AdminPage struct {
	Title string
	Board Board
	Key string
	Actor string
	Boards []Board
	Following []string
	Followers []string
	Reported []Report
	Domain string
	IsLocal bool
	PostBlacklist []PostBlacklist
	AutoSubscribe bool
}

type Report struct {
	ID string
	Count int
	Reason string
}

type Removed struct {
	ID string
	Type string
	Board string
}


type NewsItem struct {
	Title string
	Content template.HTML
	Time int
}

type PostBlacklist struct {
	Id int
	Regex string
}

func IndexGet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"mod": func(i, j int) bool { return i%j == 0 },
		"sub": func (i, j int) int { return i - j },
		"unixtoreadable": func(u int) string { return time.Unix(int64(u), 0).Format("Jan 02, 2006") }}).ParseFiles("./static/main.html", "./static/index.html"))

	actor := GetActorFromDB(db, Domain)

	var data PageData
	data.Title = "Welcome to " + actor.PreferredUsername
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = Boards
	data.Board.Name = ""
	data.Key = *Key
	data.Board.Domain = Domain
	data.Board.ModCred, _ = GetPasswordFromSession(r)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	//almost certainly there is a better algorithm for this but the old one was wrong
	//and I suck at math. This works at least.
	data.BoardRemainer = make([]int, 3-(len(data.Boards) % 3))
	if(len(data.BoardRemainer) == 3){
		data.BoardRemainer = make([]int, 0)
	}

	data.InstanceIndex = GetCollectionFromReq("https://fchan.xyz/followers").Items
	data.NewsItems = getNewsFromDB(db, 3)

	t.ExecuteTemplate(w, "layout",  data)
}

func NewsGet(w http.ResponseWriter, r *http.Request, db *sql.DB, timestamp int) {
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"sub": func (i, j int) int { return i - j },
		"unixtoreadable": func(u int) string { return time.Unix(int64(u), 0).Format("Jan 02, 2006") }}).ParseFiles("./static/main.html", "./static/news.html"))

	actor := GetActorFromDB(db, Domain)

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = Boards
	data.Board.Name = ""
	data.Key = *Key
	data.Board.Domain = Domain
	data.Board.ModCred, _ = GetPasswordFromSession(r)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.NewsItems = []NewsItem{NewsItem{}}

	var err error
	data.NewsItems[0], err = getNewsItemFromDB(db, timestamp)

	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("404 no path"))
		return
	}

	data.Title = actor.PreferredUsername + ": " + data.NewsItems[0].Title

	t.ExecuteTemplate(w, "layout",  data)
}

func AllNewsGet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"mod": func(i, j int) bool { return i%j == 0 },
		"sub": func (i, j int) int { return i - j },
		"unixtoreadable": func(u int) string { return time.Unix(int64(u), 0).Format("Jan 02, 2006") }}).ParseFiles("./static/main.html", "./static/anews.html"))

	actor := GetActorFromDB(db, Domain)

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Title = actor.PreferredUsername + " News"
	data.Boards = Boards
	data.Board.Name = ""
	data.Key = *Key
	data.Board.Domain = Domain
	data.Board.ModCred, _ = GetPasswordFromSession(r)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.NewsItems = getNewsFromDB(db, 0)

	t.ExecuteTemplate(w, "layout",  data)
}

func OutboxGet(w http.ResponseWriter, r *http.Request, db *sql.DB, collection Collection){
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"proxy": func(url string) string {
			return MediaProxy(url)
		},
		"short": func(actorName string, url string) string {
			return shortURL(actorName, url)
		},
		"parseAttachment": func(obj ObjectBase, catalog bool) template.HTML {
			return ParseAttachment(obj, catalog)
		},
		"parseContent": func(board Actor, op string, content string, thread ObjectBase) template.HTML {
			return ParseContent(db, board, op, content, thread)
		},
		"shortImg": func(url string) string {
			return  ShortImg(url)
		},
		"convertSize": func(size int64) string {
			return  ConvertSize(size)
		},
		"isOnion": func(url string) bool {
			return  IsOnion(url)
		},
		"showArchive": func() bool {
			col := GetActorCollectionDBTypeLimit(db, collection.Actor.Id, "Archive", 1)

			if len(col.OrderedItems) > 0 {
					return true
			}
			return false;
		},
		"parseReplyLink": func(actorId string, op string, id string, content string) template.HTML {
			actor := FingerActor(actorId)
			title := strings.ReplaceAll(ParseLinkTitle(actor.Id, op, content), `/\&lt;`, ">")
			link := "<a href=\"" + actor.Name + "/" + shortURL(actor.Outbox, op) + "#" + shortURL(actor.Outbox, id)  + "\" title=\"" + title + "\">&gt;&gt;" + shortURL(actor.Outbox, id) + "</a>"
			return template.HTML(link)
		},
		"add": func (i, j int) int {
			return i + j
		},
		"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/nposts.html", "./static/top.html", "./static/bottom.html", "./static/posts.html"))


	actor := collection.Actor

	postNum := r.URL.Query().Get("page")

	page, _ := strconv.Atoi(postNum)

	var returnData PageData

	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.Summary = actor.Summary
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = *actor
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.CurrentPage = page
	returnData.ReturnTo = "feed"

	returnData.Board.Post.Actor = actor.Id

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Key = *Key

	returnData.Boards = Boards
	returnData.Posts = collection.OrderedItems

	var offset = 15
	var pages []int
	pageLimit := (float64(collection.TotalItems) / float64(offset))

	if pageLimit > 11 {
		pageLimit = 11
	}

	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	returnData.Pages = pages
	returnData.TotalPage = len(returnData.Pages) - 1

	t.ExecuteTemplate(w, "layout",  returnData)
}

func CatalogGet(w http.ResponseWriter, r *http.Request, db *sql.DB, collection Collection){
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"proxy": func(url string) string {
			return MediaProxy(url)
		},
		"short": func(actorName string, url string) string {
			return shortURL(actorName, url)
		},
		"parseAttachment": func(obj ObjectBase, catalog bool) template.HTML {
			return ParseAttachment(obj, catalog)
		},
		"isOnion": func(url string) bool {
			return  IsOnion(url)
		},
		"showArchive": func() bool {
			col := GetActorCollectionDBTypeLimit(db, collection.Actor.Id, "Archive", 1)

			if len(col.OrderedItems) > 0 {
					return true
			}
			return false;
		},
		"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/ncatalog.html", "./static/top.html"))

	actor := collection.Actor

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = *actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Key = *Key
	returnData.ReturnTo = "catalog"

	returnData.Board.Post.Actor = actor.Id

	returnData.Instance = GetActorFromDB(db, Domain)

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = Boards

	returnData.Posts = collection.OrderedItems

	t.ExecuteTemplate(w, "layout",  returnData)
}

func ArchiveGet(w http.ResponseWriter, r *http.Request, db *sql.DB, collection Collection){
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"proxy": func(url string) string {
			return MediaProxy(url)
		},
		"short": func(actorName string, url string) string {
			return shortURL(actorName, url)
		},
		"shortExcerpt": func(post ObjectBase) template.HTML {
			return template.HTML(ShortExcerpt(post))
		},
		"parseAttachment": func(obj ObjectBase, catalog bool) template.HTML {
			return ParseAttachment(obj, catalog)
		},
		"mod": func(i, j int) bool { return i % j == 0 },
		"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/archive.html", "./static/bottom.html"))

	actor := collection.Actor

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = *actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Key = *Key
	returnData.ReturnTo = "archive"

	returnData.Board.Post.Actor = actor.Id

	returnData.Instance = GetActorFromDB(db, Domain)

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = Boards

	returnData.Posts = collection.OrderedItems

	t.ExecuteTemplate(w, "layout",  returnData)
}

func PostGet(w http.ResponseWriter, r *http.Request, db *sql.DB){
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"proxy": func(url string) string {
			return MediaProxy(url)
		},
		"short": func(actorName string, url string) string {
			return shortURL(actorName, url)
		},
		"parseAttachment": func(obj ObjectBase, catalog bool) template.HTML {
			return ParseAttachment(obj, catalog)
		},
		"parseContent": func(board Actor, op string, content string, thread ObjectBase) template.HTML {
			return ParseContent(db, board, op, content, thread)
		},
		"shortImg": func(url string) string {
			return  ShortImg(url)
		},
		"convertSize": func(size int64) string {
			return  ConvertSize(size)
		},
		"isOnion": func(url string) bool {
			return  IsOnion(url)
		},
		"parseReplyLink": func(actorId string, op string, id string, content string) template.HTML {
			actor := FingerActor(actorId)
			title := strings.ReplaceAll(ParseLinkTitle(actor.Id, op, content), `/\&lt;`, ">")
			link := "<a href=\"" + actor.Name + "/" + shortURL(actor.Outbox, op) + "#" + shortURL(actor.Outbox, id)  + "\" title=\"" + title + "\">&gt;&gt;" + shortURL(actor.Outbox, id) + "</a>"
			return template.HTML(link)
		},
		"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/npost.html", "./static/top.html", "./static/bottom.html", "./static/posts.html"))

	path := r.URL.Path
	actor := GetActorFromPath(db, path, "/")
	re := regexp.MustCompile("\\w+$")
	postId := re.FindString(path)

	inReplyTo := actor.Id + "/" + postId

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.ReturnTo = "feed"

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)

	returnData.Instance = GetActorFromDB(db, Domain)

	returnData.Title = "/" + returnData.Board.Name + "/ - " + returnData.Board.PrefName

	returnData.Key = *Key

	returnData.Boards = Boards

	re = regexp.MustCompile("f(\\w|[!@#$%^&*<>])+-(\\w|[!@#$%^&*<>])+")

	if re.MatchString(path) { // if non local actor post
		name := GetActorFollowNameFromPath(path)
		followActors := GetActorsFollowFromName(actor, name)
		followCollection := GetActorsFollowPostFromId(db, followActors, postId)

		if len(followCollection.OrderedItems) > 0 {
			returnData.Board.InReplyTo = followCollection.OrderedItems[0].Id
			returnData.Posts = append(returnData.Posts, followCollection.OrderedItems[0])
			var actor Actor
			actor = FingerActor(returnData.Board.InReplyTo)
			returnData.Board.Post.Actor = actor.Id
		}
	} else {
		collection := GetObjectByIDFromDB(db, inReplyTo)
		if collection.Actor != nil {
			returnData.Board.Post.Actor = collection.Actor.Id
			returnData.Board.InReplyTo = inReplyTo

			if len(collection.OrderedItems) > 0 {
				returnData.Posts = append(returnData.Posts, collection.OrderedItems[0])
			}
		}
	}

	if len(returnData.Posts) > 0 {
		returnData.PostId = shortURL(returnData.Board.To, returnData.Posts[0].Id)
	}

	t.ExecuteTemplate(w, "layout",  returnData)
}

func GetBoardCollection(db *sql.DB) []Board {
	var collection []Board
	for _, e := range FollowingBoards {
		var board Board
		boardActor := GetActorFromDB(db, e.Id)
		if boardActor.Id == "" {
			boardActor = FingerActor(e.Id)
		}
		board.Name = boardActor.Name
		board.PrefName = boardActor.PreferredUsername
		board.Location = "/" + boardActor.Name
		board.Actor = boardActor
		board.Restricted = boardActor.Restricted
		collection = append(collection, board)
	}

	sort.Sort(BoardSortAsc(collection))

	return collection
}

func WantToServePage(db *sql.DB, actorName string, page int) (Collection, bool) {

	var collection Collection
	serve := false

	if page > 10 {
		return collection, serve
	}

	actor := GetActorByNameFromDB(db, actorName)

	if actor.Id != "" {
		collection = GetObjectFromDBPage(db, actor.Id, page)
		collection.Actor = &actor
		return collection, true
	}

	return collection, serve
}

func WantToServeCatalog(db *sql.DB, actorName string) (Collection, bool) {

	var collection Collection
	serve := false

	actor := GetActorByNameFromDB(db, actorName)

	if actor.Id != "" {
		collection = GetObjectFromDBCatalog(db, actor.Id)
		collection.Actor = &actor
		return collection, true
	}

	return collection, serve
}

func WantToServeArchive(db *sql.DB, actorName string) (Collection, bool) {

	var collection Collection
	serve := false

	actor := GetActorByNameFromDB(db, actorName)

	if actor.Id != "" {
		collection = GetActorCollectionDBType(db, actor.Id, "Archive")
		collection.Actor = &actor
		return collection, true
	}

	return collection, serve
}

func StripTransferProtocol(value string) string {
	re := regexp.MustCompile("(http://|https://)?(www.)?")

	value = re.ReplaceAllString(value, "")

	return value
}

func GetCaptchaCode(captcha string) string {
	re := regexp.MustCompile("\\w+\\.\\w+$")

	code := re.FindString(captcha)

	re = regexp.MustCompile("\\w+")

	code = re.FindString(code)

	return code
}

func CreateLocalDeleteDB(db *sql.DB, id string, _type string)	{
	query := `select id from removed where id=$1`

	rows, err := db.Query(query, id)

	CheckError(err, "could not query removed")

	defer rows.Close()

	if rows.Next() {
		var i string

		rows.Scan(&i)

		if i != "" {
			query := `update removed set type=$1 where id=$2`

			_, err := db.Exec(query, _type, id)

			CheckError(err, "Could not update removed post")

		}
	} else {
		query := `insert into removed (id, type) values ($1, $2)`

		_, err := db.Exec(query, id, _type)

		CheckError(err, "Could not insert removed post")
	}
}

func GetLocalDeleteDB(db *sql.DB) []Removed {
	var deleted []Removed

	query := `select id, type from removed`

	rows, err := db.Query(query)

	CheckError(err, "could not query removed")

	defer rows.Close()

	for rows.Next() {
		var r Removed

		rows.Scan(&r.ID, &r.Type)

		deleted = append(deleted, r)
	}

	return deleted
}

func CreateLocalReportDB(db *sql.DB, id string, board string, reason string) {
	query := `select id, count from reported where id=$1 and board=$2`

	rows, err := db.Query(query, id, board)

	CheckError(err, "could not query reported")

	defer rows.Close()

	if rows.Next() {
		var i string
		var count int

		rows.Scan(&i, &count)

		if i != "" {
			count = count + 1
			query := `update reported set count=$1 where id=$2`

			_, err := db.Exec(query, count, id)

			CheckError(err, "Could not update reported post")
		}
	} else {
		query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		_, err := db.Exec(query, id, 1, board, reason)

		CheckError(err, "Could not insert reported post")
	}

}

func GetLocalReportDB(db *sql.DB, board string) []Report {
	var reported []Report

	query := `select id, count, reason from reported where board=$1`

	rows, err := db.Query(query, board)

	CheckError(err, "could not query reported")

	defer rows.Close()

	for rows.Next() {
		var r Report

		rows.Scan(&r.ID, &r.Count, &r.Reason)

		reported = append(reported, r)
	}

	return reported
}

func CloseLocalReportDB(db *sql.DB, id string, board string) {
	query := `delete from reported where id=$1 and board=$2`

	_, err := db.Exec(query, id, board)

	CheckError(err, "Could not delete local report from db")
}

func GetActorFollowNameFromPath(path string) string{
	var actor string

	re := regexp.MustCompile("f\\w+-")

	actor = re.FindString(path)

	actor = strings.Replace(actor, "f", "", 1)
	actor = strings.Replace(actor, "-", "", 1)

	return actor
}

func GetActorsFollowFromName(actor Actor, name string) []string {
	var followingActors []string
	follow := GetActorCollection(actor.Following)

	re := regexp.MustCompile("\\w+?$")

	for _, e := range follow.Items {
		if re.FindString(e.Id) == name {
			followingActors = append(followingActors, e.Id)
		}
	}

	return followingActors
}

func GetActorsFollowPostFromId(db *sql.DB, actors []string, id string) Collection{
	var collection Collection

	for _, e := range actors {
		tempCol := GetObjectByIDFromDB(db, e + "/" + id)
		if len(tempCol.OrderedItems) > 0 {
			collection = tempCol
			return collection
		}
	}

	return collection
}

type ObjectBaseSortDesc []ObjectBase
func (a ObjectBaseSortDesc) Len() int { return len(a) }
func (a ObjectBaseSortDesc) Less(i, j int) bool { return a[i].Updated > a[j].Updated }
func (a ObjectBaseSortDesc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type ObjectBaseSortAsc []ObjectBase
func (a ObjectBaseSortAsc) Len() int { return len(a) }
func (a ObjectBaseSortAsc) Less(i, j int) bool { return a[i].Published < a[j].Published }
func (a ObjectBaseSortAsc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type BoardSortAsc []Board
func (a BoardSortAsc) Len() int { return len(a) }
func (a BoardSortAsc) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a BoardSortAsc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func MediaProxy(url string) string {
	re := regexp.MustCompile("(.+)?" + Domain + "(.+)?")

	if re.MatchString(url) {
		return url
	}

	re = regexp.MustCompile("(.+)?\\.onion(.+)?")

	if re.MatchString(url) {
		return url
	}

	MediaHashs[HashMedia(url)] = url
	return "/api/media?hash=" + HashMedia(url)
}

func ParseAttachment(obj ObjectBase, catalog bool) template.HTML {

	if len(obj.Attachment) < 1 {
		return ""
	}

	var media string
	if(regexp.MustCompile(`image\/`).MatchString(obj.Attachment[0].MediaType)){
		media = "<img "
		media += "id=\"img\" "
		media += "main=\"1\" "
		media += "enlarge=\"0\" "
		media += "attachment=\"" + obj.Attachment[0].Href + "\""
		if catalog {
			media += "style=\"max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\""
		}
		if obj.Preview.Id != "" {
			media += "src=\"" + MediaProxy(obj.Preview.Href) + "\""
			media += "preview=\"" + MediaProxy(obj.Preview.Href) + "\""
		} else {
			media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\""
			media += "preview=\"" + MediaProxy(obj.Attachment[0].Href) + "\""
		}

		media += ">"

		return template.HTML(media)
	}

	if(regexp.MustCompile(`audio\/`).MatchString(obj.Attachment[0].MediaType)){
		media = "<audio "
		media += "controls=\"controls\" "
		media += "preload=\"metadta\" "
		if catalog {
			media += "style=\"margin-right: 10px; margin-bottom: 10px; max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\" "
		}
		media += ">"
		media += "<source "
		media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\" "
		media += "type=\"" + obj.Attachment[0].MediaType + "\" "
		media += ">"
		media += "Audio is not supported."
		media += "</audio>"

		return template.HTML(media)
	}

	if(regexp.MustCompile(`video\/`).MatchString(obj.Attachment[0].MediaType)){
		media = "<video "
		media += "controls=\"controls\" "
		media += "preload=\"metadta\" "
		media += "muted=\"muted\" "
		if catalog {
			media += "style=\"margin-right: 10px; margin-bottom: 10px; max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\" "
		}
		media += ">"
		media += "<source "
		media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\" "
		media += "type=\"" + obj.Attachment[0].MediaType + "\" "
		media += ">"
		media += "Video is not supported."
		media += "</video>"

		return template.HTML(media)
	}

	return template.HTML(media)
}

func ParseContent(db *sql.DB, board Actor, op string, content string, thread ObjectBase) template.HTML {

	nContent := strings.ReplaceAll(content, `<`, "&lt;")

	nContent = ParseLinkComments(db, board, op, nContent, thread)

	nContent = ParseCommentQuotes(nContent)

	nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

	return template.HTML(nContent)
};

func ParseLinkComments(db *sql.DB, board Actor, op string, content string, thread ObjectBase) string {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	//add url to each matched reply
	for i, _ := range match {
		link := strings.Replace(match[i][0], ">>", "", 1)
		isOP := ""

		domain := match[i][2]

		if link == op {
			isOP = " (OP)"
		}

		parsedLink := ConvertHashLink(domain, link)

		//formate the hover title text
		var quoteTitle string

		// if the quoted content is local get it
		// else get it from the database
		if thread.Id == link {
			quoteTitle = ParseLinkTitle(board.Outbox, op, thread.Content)
		} else {
			for _, e := range thread.Replies.OrderedItems {
				if e.Id == parsedLink {
					quoteTitle = ParseLinkTitle(board.Outbox, op, e.Content)
					break
				}
			}

			if quoteTitle == "" {
				obj := GetObjectFromDBFromID(db, parsedLink)
				if len(obj.OrderedItems) > 0 {
					quoteTitle = ParseLinkTitle(board.Outbox, op, obj.OrderedItems[0].Content)
				} else {
					quoteTitle = ParseLinkTitle(board.Outbox, op, parsedLink)
				}
			}
		}

		var style string
		if board.Restricted {
			style = "color: #af0a0f;"
		}

		//replace link with quote format
		replyID, isReply := IsReplyToOP(db, op, parsedLink)
		if isReply {
			id := shortURL(board.Outbox, replyID)

			content = strings.Replace(content, match[i][0], "<a class=\"reply\" style=\"" + style + "\" title=\"" + quoteTitle +  "\" href=\"/" + board.Name + "/" + shortURL(board.Outbox, op)  +  "#" + id + "\">&gt;&gt;" + id + ""  + isOP + "</a>", -1)

		} else {

			//this is a cross post
			parsedOP := GetReplyOP(db, parsedLink)
			actor := FingerActor(parsedLink)

			if parsedOP != "" {
				link = parsedOP + "#" + shortURL(parsedOP, parsedLink)
			}

			if actor.Id != "" {
				content = strings.Replace(content, match[i][0], "<a class=\"reply\" style=\"" + style + "\" title=\"" + quoteTitle +  "\" href=\"" + link + "\">&gt;&gt;" + shortURL(board.Outbox, parsedLink)  + isOP + " →</a>", -1)
			}
		}
	}

	return content
}

func ParseLinkTitle(actorName string, op string, content string) string {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)\w+(#.+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	for i, _ := range match {
		link := strings.Replace(match[i][0], ">>", "", 1)
		isOP := ""

		domain := match[i][2]

		if link == op {
			isOP = " (OP)"
		}

		link = ConvertHashLink(domain, link)
		content = strings.Replace(content, match[i][0], ">>" + shortURL(actorName, link)  + isOP , 1)
	}

	content = strings.ReplaceAll(content, "'", "")
	content = strings.ReplaceAll(content, "\"", "")
	content = strings.ReplaceAll(content, ">", `/\&lt;`)

	return content
}

func ParseCommentQuotes(content string) string {
	// replace quotes
	re := regexp.MustCompile(`((\r\n|\r|\n|^)>(.+)?[^\r\n])`)
	match := re.FindAllStringSubmatch(content, -1)

	for i, _ := range match {
		quote := strings.Replace(match[i][0], ">", "&gt;", 1)
		line := re.ReplaceAllString(match[i][0], "<span class=\"quote\">" + quote + "</span>")
		content = strings.Replace(content, match[i][0], line, 1)
	}

	//replace isolated greater than symboles
	re = regexp.MustCompile(`(\r\n|\n|\r)>`)

	return re.ReplaceAllString(content, "\r\n<span class=\"quote\">&gt;</span>")
}

func ConvertHashLink(domain string, link string) string {
	re := regexp.MustCompile(`(#.+)`)
	parsedLink := re.FindString(link)

	if parsedLink != "" {
		parsedLink = domain + "" + strings.Replace(parsedLink, "#", "", 1)
		parsedLink = strings.Replace(parsedLink, "\r", "", -1)
	} else {
		parsedLink = link
	}

	return parsedLink
}

func ShortImg(url string) string {
	nURL := url

	re := regexp.MustCompile(`(\.\w+$)`)

	fileName := re.ReplaceAllString(url, "")

	if(len(fileName) > 26) {
		re := regexp.MustCompile(`(^.{26})`)

		match := re.FindStringSubmatch(fileName)

		if len(match) > 0 {
			nURL = match[0]
		}

		re = regexp.MustCompile(`(\..+$)`)

		match = re.FindStringSubmatch(url)

		if len(match) > 0 {
			nURL = nURL  + "(...)" + match[0];
		}
	}

	return nURL;
}

func ConvertSize(size int64) string {
	var rValue string

	convert := float32(size) / 1024.0;

	if(convert > 1024) {
		convert = convert / 1024.0;
		rValue = fmt.Sprintf("%.2f MB", convert)
	} else {
		rValue = fmt.Sprintf("%.2f KB", convert)
	}

		return rValue;
}

func ShortExcerpt(post ObjectBase) string {
	var returnString string

	if post.Name != "" {
		returnString = post.Name + ": " + post.Content;
	} else {
		returnString = post.Content;
	}

	re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

	match := re.FindStringSubmatch(returnString)

	if len(match) > 0 {
		returnString = match[0] + "..."
	}

	re = regexp.MustCompile(`(^.+:)`)

	match = re.FindStringSubmatch(returnString)

	if len(match) > 0 {
		returnString = strings.Replace(returnString, match[0], "<b>" + match[0] + "</b>", 1)
	}

	return returnString
}

func IsOnion(url string) bool {
	re := regexp.MustCompile(`\.onion`)
	if(re.MatchString(url)) {
		return true;
	}

	return false
}
