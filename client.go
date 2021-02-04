package main

import "fmt"
import "net/http"
import "html/template"
import "database/sql"
import _ "github.com/lib/pq"
import "strings"
import "strconv"
import "sort"
import "regexp"
import "io/ioutil"
import "encoding/json"
import "os"

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
}

type PageData struct {
	Title string
	Message string	
	Board Board
	Pages []int
	CurrentPage int
	TotalPage int
	Boards []Board
	Posts []ObjectBase
	Key string
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
}

type Report struct {
	ID string
	Count int
}

type Removed struct {
	ID string
	Type string
	Board string
}

func IndexGet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	t := template.Must(template.ParseFiles("./static/main.html", "./static/index.html"))

	actor := GetActorFromDB(db, Domain)
	
	var data PageData
	data.Title = "Welcome to " + actor.PreferredUsername
	data.Message = fmt.Sprintf("%s is a federated image board based on activitypub. The current version of the code running the server is still a work in progress, expect a bumpy ride for the time being. Get the server code here https://github.com/FChannel0", Domain)
	data.Boards = Boards
	data.Board.Name = ""
	data.Key = *Key
	data.Board.Domain = Domain
	data.Board.ModCred, _ = GetPasswordFromSession(r)

	t.ExecuteTemplate(w, "layout",  data)	
}

func OutboxGet(w http.ResponseWriter, r *http.Request, db *sql.DB, collection Collection){

	t := template.Must(template.ParseFiles("./static/main.html", "./static/nposts.html", "./static/top.html", "./static/bottom.html", "./static/posts.html"))	

	actor := collection.Actor

	postNum := strings.Replace(r.URL.EscapedPath(), "/" + actor.Name + "/", "", 1)

	page, _ := strconv.Atoi(postNum)
	
	var returnData PageData

	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.Summary = actor.Summary
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor.Id = actor.Id
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.CurrentPage = page

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Key = *Key	

	var mergeCollection Collection

	for _, e := range collection.OrderedItems {
		if e.Type != "Tombstone" {
			mergeCollection.OrderedItems = append(mergeCollection.OrderedItems, e)
		}
	}

	domainURL := GetDomainURL(*actor)
	
	if domainURL == Domain {
		followCol := GetObjectsFromFollow(db, *actor)
		for _, e := range followCol {
			if e.Type != "Tombstone" {
				mergeCollection.OrderedItems = append(mergeCollection.OrderedItems, e)
			}
		}
	}
	
	DeleteRemovedPosts(db, &mergeCollection)
	DeleteTombstoneReplies(&mergeCollection)

	for i, _ := range mergeCollection.OrderedItems {
		sort.Sort(ObjectBaseSortAsc(mergeCollection.OrderedItems[i].Replies.OrderedItems))
	}

	DeleteTombstonePosts(&mergeCollection)
	sort.Sort(ObjectBaseSortDesc(mergeCollection.OrderedItems))	

	returnData.Boards = Boards

	offset := 8
	start := page * offset
	for i := 0; i < offset; i++ {
		length := len(mergeCollection.OrderedItems)
		current := start + i
		if(current < length) {
			returnData.Posts = append(returnData.Posts, mergeCollection.OrderedItems[current])
		}
	}

	for i, e := range returnData.Posts {
		var replies []ObjectBase
		for i := 0; i < 5; i++ {
			cur := len(e.Replies.OrderedItems) - i - 1
			if cur > -1 {
				replies = append(replies, e.Replies.OrderedItems[cur])
			}
		}

		var orderedReplies []ObjectBase
		for i := 0; i < 5; i++ {
			cur := len(replies) - i - 1
			if cur > - 1 {
				orderedReplies = append(orderedReplies, replies[cur])
			}
		}

		for _, e := range returnData.Posts[i].Replies.OrderedItems {
			if len(e.Attachment) > 0 {
				returnData.Posts[i].Replies.TotalImgs = returnData.Posts[i].Replies.TotalImgs + 1
			}
		}				
		
		returnData.Posts[i].Replies.TotalItems = len(returnData.Posts[i].Replies.OrderedItems)		
		returnData.Posts[i].Replies.OrderedItems = orderedReplies
	}

	var pages []int
	pageLimit := (float64(len(mergeCollection.OrderedItems)) / float64(offset))
	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	returnData.Pages = pages
	returnData.TotalPage = len(returnData.Pages) - 1

	t.ExecuteTemplate(w, "layout",  returnData)
}

func CatalogGet(w http.ResponseWriter, r *http.Request, db *sql.DB, collection Collection){

	t := template.Must(template.ParseFiles("./static/main.html", "./static/ncatalog.html", "./static/top.html"))			
	
	actor := collection.Actor

	var mergeCollection Collection

	for _, e := range collection.OrderedItems {
		mergeCollection.OrderedItems = append(mergeCollection.OrderedItems, e)
	}

	domainURL := GetDomainURL(*actor)

	if domainURL == Domain {
		followCol := GetObjectsFromFollow(db, *actor)	
		for _, e := range followCol {
			if e.Type != "Tombstone" {			
				mergeCollection.OrderedItems = append(mergeCollection.OrderedItems, e)
			}
		}
	}

	DeleteRemovedPosts(db, &mergeCollection)
	DeleteTombstonePosts(&mergeCollection)
	
	sort.Sort(ObjectBaseSortDesc(mergeCollection.OrderedItems))
	
	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor.Id = actor.Id
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Key = *Key

	returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
	returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)	

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = Boards

	returnData.Posts = mergeCollection.OrderedItems

	for i, _ := range returnData.Posts {
		for _, e := range returnData.Posts[i].Replies.OrderedItems {
			if len(e.Attachment) > 0 {
				returnData.Posts[i].Replies.TotalImgs = returnData.Posts[i].Replies.TotalImgs + 1
			}
		}		
		returnData.Posts[i].Replies.TotalItems = len(returnData.Posts[i].Replies.OrderedItems)		
	}

	t.ExecuteTemplate(w, "layout",  returnData)
}

func PostGet(w http.ResponseWriter, r *http.Request, db *sql.DB){

	t := template.Must(template.ParseFiles("./static/main.html", "./static/npost.html", "./static/top.html", "./static/bottom.html", "./static/posts.html"))
	
	path := r.URL.Path
	actor := GetActorFromPath(db, path, "/")
	re := regexp.MustCompile("\\w+$")
	postId := re.FindString(path)

	inReplyTo := actor.Id + "/" + postId

	var returnData PageData

	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor.Id = actor.Id
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = GetPasswordFromSession(r)
	returnData.Board.Domain = Domain
	returnData.Board.Restricted = actor.Restricted


	if GetDomainURL(actor) != "" {
		returnData.Board.Captcha = Domain + "/" + GetRandomCaptcha(db)
		returnData.Board.CaptchaCode = GetCaptchaCode(returnData.Board.Captcha)
	}

	returnData.Title = "/" + returnData.Board.Name + "/ - " + returnData.Board.PrefName

	returnData.Key = *Key	

	returnData.Boards = Boards

	re = regexp.MustCompile("f\\w+-\\w+")

	if re.MatchString(path) {
		name := GetActorFollowNameFromPath(path)
		followActors := GetActorsFollowFromName(actor, name)
		followCollection := GetActorsFollowPostFromId(db, followActors, postId)
		
		DeleteRemovedPosts(db, &followCollection)
		DeleteTombstoneReplies(&followCollection)

		for i, _ := range followCollection.OrderedItems {
			sort.Sort(ObjectBaseSortAsc(followCollection.OrderedItems[i].Replies.OrderedItems))
		}

		if len(followCollection.OrderedItems) > 0 {		
			returnData.Board.InReplyTo = followCollection.OrderedItems[0].Id
			returnData.Posts = append(returnData.Posts, followCollection.OrderedItems[0])
			sort.Sort(ObjectBaseSortAsc(returnData.Posts[0].Replies.OrderedItems))			
		}

	} else {
		returnData.Board.InReplyTo = inReplyTo
		collection := GetActorCollectionByID(db, inReplyTo)

		DeleteRemovedPosts(db, &collection)

		for i, e := range collection.OrderedItems {
			var replies CollectionBase
			for _, k := range e.Replies.OrderedItems {
				if k.Type != "Tombstone" {
					replies.OrderedItems = append(replies.OrderedItems, k)
				} else {
					collection.OrderedItems[i].Replies.TotalItems = collection.OrderedItems[i].Replies.TotalItems - 1
					if k.Preview.Id != "" {
						collection.OrderedItems[i].Replies.TotalImgs = collection.OrderedItems[i].Replies.TotalImgs - 1
					}
				}
			}
			collection.TotalItems = collection.OrderedItems[i].Replies.TotalItems
			collection.TotalImgs = collection.OrderedItems[i].Replies.TotalImgs			
			collection.OrderedItems[i].Replies = &replies
			sort.Sort(ObjectBaseSortAsc(e.Replies.OrderedItems))
		}				

		if len(collection.OrderedItems) > 0 {
			returnData.Posts = append(returnData.Posts, collection.OrderedItems[0])
			sort.Sort(ObjectBaseSortAsc(returnData.Posts[0].Replies.OrderedItems))
		}
	}

	for i, _ := range returnData.Posts {
		for _, e := range returnData.Posts[i].Replies.OrderedItems {
			if len(e.Attachment) > 0 {
				returnData.Posts[i].Replies.TotalImgs = returnData.Posts[i].Replies.TotalImgs + 1
			}
		}
		returnData.Posts[i].Replies.TotalItems = len(returnData.Posts[i].Replies.OrderedItems)		
	}	

	t.ExecuteTemplate(w, "layout",  returnData)			
}

func GetRemoteActor(id string) Actor {

	var respActor Actor

	id = StripTransferProtocol(id)

	req, err := http.NewRequest("GET", "http://" + id, nil)

	CheckError(err, "error with getting actor req")

	req.Header.Set("Accept", activitystreams)

	resp, err := http.DefaultClient.Do(req)

	if err != nil || resp.StatusCode != 200 {
		fmt.Println("could not get actor from " + id)		
		return respActor
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &respActor)

	CheckError(err, "error getting actor from body")

	return respActor
}

func GetBoardCollection(db *sql.DB) []Board {
	var collection []Board
	for _, e := range FollowingBoards {
		var board Board
		boardActor := GetActorFromDB(db, e.Id)
		if boardActor.Id == "" {
			boardActor = GetRemoteActor(e.Id)
		}
		board.Name = "/" + boardActor.Name + "/"
		board.PrefName = boardActor.PreferredUsername
		board.Location = "/" + boardActor.Name
		board.Actor = boardActor
		collection = append(collection, board)
	}

	sort.Sort(BoardSortAsc(collection))
	
	return collection
}

func WantToServe(db *sql.DB, actorName string) (Collection, bool) {

	var collection Collection
	serve := false

	boardActor := GetActorByNameFromDB(db, actorName)

	if boardActor.Id != "" {
		collection = GetActorCollectionDB(db, boardActor)
		return collection, true
	}
	
	for _, e := range FollowingBoards {
		boardActor := GetActorFromDB(db, e.Id)
		
		if boardActor.Id == "" {
			boardActor = GetActor(e.Id)
		}

		if boardActor.Name == actorName {
			serve = true
			if IsActorLocal(db, boardActor.Id) {
				collection = GetActorCollectionDB(db, boardActor)
			} else {
				collection = GetActorCollectionCache(db, boardActor)
			}
			collection.Actor = &boardActor
			return collection, serve
		}
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

func GetDomainURL(actor Actor) string {
	re := regexp.MustCompile("(https://|http://)?(www)?.+/")

	domainURL := re.FindString(actor.Id)

	re = regexp.MustCompile("/$")

	domainURL = re.ReplaceAllString(domainURL, "")

	return domainURL
}

func DeleteTombstoneReplies(collection *Collection) {

	for i, e := range collection.OrderedItems {
		var replies CollectionBase
		for _, k := range e.Replies.OrderedItems {
			if k.Type != "Tombstone" {
				replies.OrderedItems = append(replies.OrderedItems, k)
			}				
		}
		
		replies.TotalItems = collection.OrderedItems[i].Replies.TotalItems
		replies.TotalImgs = collection.OrderedItems[i].Replies.TotalImgs
		collection.OrderedItems[i].Replies = &replies
	}			
}

func DeleteTombstonePosts(collection *Collection) {
	var nColl Collection
	
	for _, e := range collection.OrderedItems {
		if e.Type != "Tombstone" {
			nColl.OrderedItems = append(nColl.OrderedItems, e)
		}
	}
	collection.OrderedItems = nColl.OrderedItems
}

func DeleteRemovedPosts(db *sql.DB, collection *Collection) {

	removed := GetLocalDeleteDB(db)

	for p, e := range collection.OrderedItems {
		for _, j := range removed {
			if e.Id == j.ID {
				if j.Type == "attachment" {
					collection.OrderedItems[p].Preview.Href = "/public/removed.png"
					collection.OrderedItems[p].Preview.Name = "deleted"
					collection.OrderedItems[p].Preview.MediaType = "image/png"											
					collection.OrderedItems[p].Attachment[0].Href = "/public/removed.png"
					collection.OrderedItems[p].Attachment[0].Name = "deleted"					
					collection.OrderedItems[p].Attachment[0].MediaType = "image/png"
				} else {
					collection.OrderedItems[p].AttributedTo = "deleted"
					collection.OrderedItems[p].Content = ""
					collection.OrderedItems[p].Type = "Tombstone"
					if collection.OrderedItems[p].Attachment != nil {
						collection.OrderedItems[p].Preview.Href = "/public/removed.png"
						collection.OrderedItems[p].Preview.Name = "deleted"
						collection.OrderedItems[p].Preview.MediaType = "image/png"						
						collection.OrderedItems[p].Attachment[0].Href = "/public/removed.png"
						collection.OrderedItems[p].Attachment[0].Name = "deleted"
						collection.OrderedItems[p].Attachment[0].MediaType = "image/png"
					}
				}
			}
		}
		
		for i, r := range e.Replies.OrderedItems {
			for _, k := range removed {
				if r.Id == k.ID {
					if k.Type == "attachment" {
						e.Replies.OrderedItems[i].Preview.Href = "/public/removed.png"
						e.Replies.OrderedItems[i].Preview.Name = "deleted"						
						e.Replies.OrderedItems[i].Preview.MediaType = "image/png"													
						e.Replies.OrderedItems[i].Attachment[0].Href = "/public/removed.png"
						e.Replies.OrderedItems[i].Attachment[0].Name = "deleted"
						e.Replies.OrderedItems[i].Attachment[0].MediaType = "image/png"
						collection.OrderedItems[p].Replies.TotalImgs = collection.OrderedItems[p].Replies.TotalImgs - 1
					} else {
						e.Replies.OrderedItems[i].AttributedTo = "deleted"
						e.Replies.OrderedItems[i].Content = ""
						e.Replies.OrderedItems[i].Type = "Tombstone"
						if e.Replies.OrderedItems[i].Attachment != nil {
							e.Replies.OrderedItems[i].Preview.Href = "/public/removed.png"
							e.Replies.OrderedItems[i].Preview.Name = "deleted"
							e.Replies.OrderedItems[i].Preview.MediaType = "image/png"							
							e.Replies.OrderedItems[i].Attachment[0].Name = "deleted"												
							e.Replies.OrderedItems[i].Attachment[0].Href = "/public/removed.png"						
							e.Replies.OrderedItems[i].Attachment[0].MediaType = "image/png"
							collection.OrderedItems[p].Replies.TotalImgs = collection.OrderedItems[p].Replies.TotalImgs - 1							
						}
						collection.OrderedItems[p].Replies.TotalItems = collection.OrderedItems[p].Replies.TotalItems - 1
					}
				}
			}
		}
	}
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
	
	query := `select id, count from reported where board=$1`

	rows, err := db.Query(query, board)

	CheckError(err, "could not query reported")

	defer rows.Close()

	for rows.Next() {
		var r Report

		rows.Scan(&r.ID, &r.Count)

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
		tempCol := GetActorCollectionByID(db, e + "/" + id)
		if len(tempCol.OrderedItems) > 0 {
			collection = tempCol
		}
	}

	return collection
}

func GetActorCollectionByID(db *sql.DB, postID string) Collection {
	collection := GetObjectByIDFromDB(db, postID)
	if len(collection.OrderedItems) < 1 {
		collection = GetObjectByIDFromCache(db, postID)
	}

	return collection
}

func CreateClientKey() string{

	file, err := os.Create("clientkey")

	CheckError(err, "could not create client key in file")

	defer file.Close()

	key := CreateKey(32)
	file.WriteString(key)
	return key
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
