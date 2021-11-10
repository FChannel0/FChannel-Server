package db

import (
	"database/sql"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

// TODO: merge these
var db *sql.DB
var DB *sql.DB

type NewsItem struct {
	Title   string
	Content template.HTML
	Time    int
}

// ConnectDB connects to the PostgreSQL database configured.
func ConnectDB() error {
	host := config.DBHost
	port := config.DBPort
	user := config.DBUser
	password := config.DBPassword
	dbname := config.DBName

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	_db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return err
	}

	if err := _db.Ping(); err != nil {
		return err
	}

	fmt.Println("Successfully connected DB")
	db = _db
	DB = _db

	return nil
}

// Close closes the database connection.
func Close() error {
	return db.Close()
}

func CreateUniqueID(actor string) (string, error) {
	var newID string
	isUnique := false
	for !isUnique {
		newID = util.RandomID(8)

		query := "select id from activitystream where id=$1"
		args := fmt.Sprintf("%s/%s/%s", config.Domain, actor, newID)
		rows, err := db.Query(query, args)
		if err != nil {
			return "", err
		}

		defer rows.Close()

		// reusing a variable here
		// if we encounter a match, it'll get set to false causing the outer for loop to loop and to go through this all over again
		// however if nothing is there, it'll remain true and exit the loop
		isUnique = true
		for rows.Next() {
			isUnique = false
			break
		}
	}

	return newID, nil
}

func RunDatabaseSchema() error {
	query, err := ioutil.ReadFile("databaseschema.psql")
	if err != nil {
		return err
	}

	_, err = db.Exec(string(query))
	return err
}

func GetActorFromDB(id string) (activitypub.Actor, error) {
	var nActor activitypub.Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where id=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return nActor, err
	}

	var publicKeyPem string
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem); err != nil {
			return nActor, err
		}
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)
	if err != nil {
		return nActor, err
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, err
		}
	}

	return nActor, nil
}

func GetActorByNameFromDB(name string) (activitypub.Actor, error) {
	var nActor activitypub.Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where name=$1`

	rows, err := db.Query(query, name)
	if err != nil {
		return nActor, err
	}

	var publicKeyPem string
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem); err != nil {
			return nActor, err
		}
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, err
		}
	}

	return nActor, nil
}

func CreateNewBoardDB(actor activitypub.Actor) (activitypub.Actor, error) {
	query := `insert into actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary, restricted) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := db.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted)

	if err != nil {
		// TODO: board exists error
		return activitypub.Actor{}, err
	} else {
		fmt.Println("board added")

		for _, e := range actor.AuthRequirement {
			query = `insert into actorauth (type, board) values ($1, $2)`

			if _, err := db.Exec(query, e, actor.Name); err != nil {
				return activitypub.Actor{}, err
			}
		}

		var verify Verify

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "admin"

		CreateVerification(verify)

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "janitor"

		CreateVerification(verify)

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "post"

		CreateVerification(verify)

		var nverify Verify
		nverify.Board = actor.Id
		nverify.Identifier = "admin"
		nverify.Type = "admin"
		CreateBoardMod(nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "janitor"
		nverify.Type = "janitor"
		CreateBoardMod(nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "post"
		nverify.Type = "post"
		CreateBoardMod(nverify)

		CreatePem(actor)

		if actor.Name != "main" {
			var nObject activitypub.ObjectBase
			var nActivity activitypub.Activity

			nActor, err := GetActorFromDB(config.Domain)
			if err != nil {
				return actor, err
			}

			nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			nActivity.Type = "Follow"
			nActivity.Actor = &nActor
			nActivity.Object = &nObject

			mActor, err := GetActorFromDB(actor.Id)
			if err != nil {
				return actor, err
			}

			nActivity.Object.Actor = mActor.Id
			nActivity.To = append(nActivity.To, actor.Id)

			response := AcceptFollow(nActivity)
			if _, err := SetActorFollowingDB(response); err != nil {
				return actor, err
			}
			if err := MakeActivityRequest(nActivity); err != nil {
				return actor, err
			}
		}

	}

	return actor, nil
}

func GetBoards() ([]activitypub.Actor, error) {
	var board []activitypub.Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers FROM actor`

	rows, err := db.Query(query)
	if err != nil {
		return board, err
	}

	defer rows.Close()
	for rows.Next() {
		var actor = new(activitypub.Actor)

		if err := rows.Scan(&actor.Type, &actor.Id, &actor.Name, &actor.PreferredUsername, &actor.Inbox, &actor.Outbox, &actor.Following, &actor.Followers); err != nil {
			return board, err
		}

		board = append(board, *actor)
	}

	return board, nil
}

func WriteObjectToDB(obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	id, err := CreateUniqueID(obj.Actor)
	if err != nil {
		return obj, err
	}

	obj.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			id, err := CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, err
			}

			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Preview.Published = time.Now().UTC()
			obj.Preview.Updated = time.Now().UTC()
			obj.Preview.AttributedTo = obj.Id
			if err := WritePreviewToDB(*obj.Preview); err != nil {
				return obj, err
			}
		}

		for i := range obj.Attachment {
			id, err := CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, err
			}

			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Attachment[i].Published = time.Now().UTC()
			obj.Attachment[i].Updated = time.Now().UTC()
			obj.Attachment[i].AttributedTo = obj.Id
			WriteAttachmentToDB(obj.Attachment[i])
			WriteActivitytoDBWithAttachment(obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		if err := WriteActivitytoDB(obj); err != nil {
			return obj, err
		}
	}

	if err := WriteObjectReplyToDB(obj); err != nil {
		return obj, err
	}

	err = WriteWalletToDB(obj)
	return obj, err
}

func WriteObjectUpdatesToDB(obj activitypub.ObjectBase) error {
	query := `update activitystream set updated=$1 where id=$2`

	if _, err := db.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id); err != nil {
		return err
	}

	query = `update cacheactivitystream set updated=$1 where id=$2`

	_, err := db.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id)
	return err
}

func WriteObjectReplyToLocalDB(id string, replyto string) error {
	query := `select id from replies where id=$1 and inreplyto=$2`

	rows, err := db.Query(query, id, replyto)
	if err != nil {
		return err
	}
	defer rows.Close()

	var nID string
	rows.Next()
	rows.Scan(&nID)

	if nID == "" {
		query := `insert into replies (id, inreplyto) values ($1, $2)`

		if _, err := db.Exec(query, id, replyto); err != nil {
			return err
		}
	}

	query = `select inreplyto from replies where id=$1`

	rows, err = db.Query(query, replyto)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var val string
		rows.Scan(&val)
		if val == "" {
			updated := time.Now().UTC().Format(time.RFC3339)
			query := `update activitystream set updated=$1 where id=$2`

			if _, err := db.Exec(query, updated, replyto); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteObjectReplyToDB(obj activitypub.ObjectBase) error {
	for i, e := range obj.InReplyTo {

		if res, err := CheckIfObjectOP(obj.Id); err == nil && !res && i == 0 {
			nType, err := GetObjectTypeDB(e.Id)
			if err != nil {
				return err
			}

			if nType == "Archive" {
				if err := UpdateObjectTypeDB(obj.Id, "Archive"); err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}

		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, e.Id)
		if err != nil {
			return err
		}
		defer rows.Close()

		var id string
		rows.Next()
		if err := rows.Scan(&id); err != nil {
			return err
		}

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := db.Exec(query, obj.Id, e.Id)
			if err != nil {
				return err
			}
		}

		update := true
		for _, e := range obj.Option {
			if e == "sage" || e == "nokosage" {
				update = false
				break
			}
		}

		if update {
			if err := WriteObjectUpdatesToDB(e); err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, "")
		if err != nil {
			return err
		}
		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := db.Exec(query, obj.Id, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteActorObjectReplyToDB(obj activitypub.ObjectBase) error {
	for _, e := range obj.InReplyTo {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, e.Id)
		if err != nil {
			return err
		}

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := db.Exec(query, obj.Id, e.Id); err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := db.Query(query, obj.Id, "")
		if err != nil {
			return err
		}
		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := db.Exec(query, obj.Id, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteWalletToDB(obj activitypub.ObjectBase) error {
	for _, e := range obj.Option {
		if e == "wallet" {
			for _, e := range obj.Wallet {
				query := `insert into wallet (id, type, address) values ($1, $2, $3)`

				if _, err := db.Exec(query, obj.Id, e.Type, e.Address); err != nil {
					return err
				}
			}

			return nil
		}
	}
	return nil
}

func WriteActivitytoDB(obj activitypub.ObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteActivitytoDBWithAttachment(obj activitypub.ObjectBase, attachment activitypub.ObjectBase, preview activitypub.NestedObjectBase) {

	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, e := db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	if e != nil {
		fmt.Println("error inserting new activity with attachment")
		panic(e)
	}
}

func WriteAttachmentToDB(obj activitypub.ObjectBase) {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, e := db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)

	if e != nil {
		fmt.Println("error inserting new attachment")
		panic(e)
	}
}

func WritePreviewToDB(obj activitypub.NestedObjectBase) error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}

func GetActivityFromDB(id string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var nActor activitypub.Actor
	var result []activitypub.ObjectBase

	nColl.Actor = &nActor

	query := `select  actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from  activitystream where id=$1 order by updated asc`

	rows, err := db.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&nColl.Actor.Id, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		var err error
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromDBPage(id string, page int) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select count (x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 15 offset $2`

	rows, err := db.Query(query, id, page*15)
	if err != nil {
		return nColl, err
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		var err error
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDBLimit(post, 5)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.TotalItems = count
	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorObjectCollectionFromDB(actorId string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`

	rows, err := db.Query(query, actorId)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromDB(id string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 order by updated desc`

	rows, err := db.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromDBFromID(id string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id like $1 and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id like $1 and type='Note') as x order by x.updated`

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(id)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		id = re.ReplaceAllString(id, "")
		id = "%" + match[1] + "/" + id
	}

	rows, err := db.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromDBCatalog(id string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 165`

	rows, err := db.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies activitypub.CollectionBase

		post.Replies = &replies

		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectByIDFromDB(postID string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 and (type='Note' or type='Archive') union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 and (type='Note' or type='Archive')) as x`

	rows, err := db.Query(query, postID)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		actor, err = GetActorFromDB(actor.Id)
		if err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		nColl.Actor = &actor

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetInReplyToDB(parent activitypub.ObjectBase) ([]activitypub.ObjectBase, error) {
	var result []activitypub.ObjectBase

	query := `select inreplyto from replies where id =$1`

	rows, err := db.Query(query, parent.Id)
	if err != nil {
		return result, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase

		if err := rows.Scan(&post.Id); err != nil {
			return result, err
		}

		result = append(result, post)
	}

	return result, nil
}

func GetObjectRepliesDBLimit(parent activitypub.ObjectBase, limit int) (*activitypub.CollectionBase, int, int, error) {
	var nColl activitypub.CollectionBase
	var result []activitypub.ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published desc limit $2`

	rows, err := db.Query(query, parent.Id, limit)
	if err != nil {
		return nil, 0, 0, err
	}

	var postCount int
	var attachCount int

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt, err = GetObjectRepliesRepliesDB(post)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	sort.Sort(activitypub.ObjectBaseSortAsc(nColl.OrderedItems))

	return &nColl, postCount, attachCount, nil
}

func GetObjectRepliesDB(parent activitypub.ObjectBase) (*activitypub.CollectionBase, int, int, error) {
	var nColl activitypub.CollectionBase
	var result []activitypub.ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`

	rows, err := db.Query(query, parent.Id)
	if err != nil {
		return nil, 0, 0, err
	}

	var postCount int
	var attachCount int

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt, err = GetObjectRepliesRepliesDB(post)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, postCount, attachCount, nil
}

func GetObjectRepliesReplies(parent activitypub.ObjectBase) (*activitypub.CollectionBase, int, int, error) {
	var nColl activitypub.CollectionBase
	var result []activitypub.ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') order by updated asc`

	rows, err := db.Query(query, parent.Id)
	if err != nil {
		return &nColl, 0, 0, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, 0, 0, err
		}

		post.Actor = actor.Id

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, 0, 0, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, 0, 0, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, 0, 0, nil
}

func GetObjectRepliesRepliesDB(parent activitypub.ObjectBase) (*activitypub.CollectionBase, int, int, error) {
	var nColl activitypub.CollectionBase
	var result []activitypub.ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published asc`

	rows, err := db.Query(query, parent.Id)
	if err != nil {
		return &nColl, 0, 0, err
	}

	var postCount int
	var attachCount int
	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, postCount, attachCount, nil
}

func CheckIfObjectOP(id string) (bool, error) {
	var count int

	query := `select count(id) from replies where inreplyto='' and id=$1 `

	rows, err := db.Query(query, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&count); err != nil {
		return false, err
	}

	if count > 0 {
		return true, nil
	}

	return false, nil
}

func GetObjectRepliesCount(parent activitypub.ObjectBase) (int, int, error) {
	var countId int
	var countImg int

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over() from (select id, attachment from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select id, attachment from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x`

	rows, err := db.Query(query, parent.Id)
	if err != nil {
		return 0, 0, err
	}

	defer rows.Close()

	rows.Next()
	err = rows.Scan(&countId, &countImg)

	return countId, countImg, err
}

func GetObjectAttachment(id string) ([]activitypub.ObjectBase, error) {
	var attachments []activitypub.ObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`

	rows, err := db.Query(query, id)
	if err != nil {
		return attachments, err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment = new(activitypub.ObjectBase)

		if err := rows.Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published); err != nil {
			return attachments, err
		}

		attachments = append(attachments, *attachment)
	}

	return attachments, nil
}

func GetObjectPreview(id string) (*activitypub.NestedObjectBase, error) {
	var preview activitypub.NestedObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`

	rows, err := db.Query(query, id)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published); err != nil {
			return nil, err
		}
	}

	return &preview, nil
}

func GetObjectPostsTotalDB(actor activitypub.Actor) (int, error) {
	count := 0
	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`

	rows, err := db.Query(query, actor.Id)
	if err != nil {
		return count, err
	}

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func GetObjectImgsTotalDB(actor activitypub.Actor) (int, error) {
	count := 0
	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`

	rows, err := db.Query(query, actor.Id)
	if err != nil {
		return count, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func DeletePreviewFromFile(id string) error {
	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`

	rows, err := db.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var href string

		if err := rows.Scan(&href); err != nil {
			return err
		}

		href = strings.Replace(href, config.Domain+"/", "", 1)

		if href != "static/notfound.png" {
			_, err = os.Stat(href)
			if err == nil {
				return os.Remove(href)
			}
			return err
		}
	}

	return nil
}

func RemovePreviewFromFile(id string) error {
	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`
	rows, err := db.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var href string

		if err := rows.Scan(&href); err != nil {
			return err
		}

		href = strings.Replace(href, config.Domain+"/", "", 1)

		if href != "static/notfound.png" {
			_, err = os.Stat(href)
			if err == nil {
				return os.Remove(href)
			}
			return err
		}
	}

	return DeletePreviewFromDB(id)
}

func DeleteAttachmentFromFile(id string) error {
	query := `select href from activitystream where id in (select attachment from activitystream where id=$1)`

	rows, err := db.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var href string

		if err := rows.Scan(&href); err != nil {
			return err
		}

		href = strings.Replace(href, config.Domain+"/", "", 1)

		if href != "static/notfound.png" {
			_, err = os.Stat(href)
			if err == nil {
				os.Remove(href)
			}
			return err
		}
	}

	return nil
}

func TombstonePreviewRepliesFromDB(id string) error {
	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`

	rows, err := db.Query(query, id)
	if err != nil {
		return err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment string

		if err := rows.Scan(&attachment); err != nil {
			return err
		}

		if err := DeletePreviewFromFile(attachment); err != nil {
			return err
		}

		if err := TombstonePreviewFromDB(attachment); err != nil {
			return err
		}
	}

	return nil
}

func TombstoneAttachmentRepliesFromDB(id string) error {
	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`

	rows, err := db.Query(query, id)
	if err != nil {
		return err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment string

		if err := rows.Scan(&attachment); err != nil {
			return err
		}

		if err := DeleteAttachmentFromFile(attachment); err != nil {
			return err
		}

		if err := TombstoneAttachmentFromDB(attachment); err != nil {
			return err
		}
	}

	return nil
}

func TombstoneAttachmentFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from activitystream where id=$3)`

	if _, err := db.Exec(query, config.Domain+"/static/notfound.png", datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from cacheactivitystream where id=$3)`

	_, err := db.Exec(query, config.Domain+"/static/notfound.png", datetime, id)
	return err
}

func DeleteAttachmentFromDB(id string) error {
	query := `delete from activitystream where id in (select attachment from activitystream where id=$1)`

	if _, err := db.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id in (select attachment from cacheactivitystream where id=$1)`

	_, err := db.Exec(query, id)
	return err
}

func TombstonePreviewFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from activitystream where id=$3)`

	if _, err := db.Exec(query, config.Domain+"/static/notfound.png", datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err := db.Exec(query, config.Domain+"/static/notfound.png", datetime, id)
	return err
}

func DeletePreviewFromDB(id string) error {
	query := `delete from activitystream  where id=$1`

	if _, err := db.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id in (select preview from cacheactivitystream where id=$1)`

	_, err := db.Exec(query, id)
	return err
}

func DeleteObjectRepliedTo(id string) error {
	query := `delete from replies where id=$1`
	_, err := db.Exec(query, id)
	return err
}

func TombstoneObjectFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)
	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id=$2`

	if _, err := db.Exec(query, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='',  deleted=$1 where id=$2`

	_, err := db.Exec(query, datetime, id)
	return err
}

func DeleteObjectFromDB(id string) error {
	var query = `delete from activitystream where id=$1`

	if _, err := db.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id=$1`

	_, err := db.Exec(query, id)
	return err
}

func DeleteObjectsInReplyTo(id string) error {
	query := `delete from replies where id in (select id from replies where inreplyto=$1)`

	_, err := db.Exec(query, id)
	return err
}

func TombstoneObjectRepliesFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	if _, err := db.Exec(query, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	_, err := db.Exec(query, datetime, id)
	return err
}

func SetAttachmentFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id=$3)`

	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream  where id=$3)`

	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetAttachmentRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id in (select id from replies where inreplyto=$3))`

	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream where id in (select id from replies where inreplyto=$3))`

	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetPreviewFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id=$3)`

	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetPreviewRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id in (select id from replies where inreplyto=$3))`

	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id in (select id from replies where inreplyto=$3))`

	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetObjectFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id=$3`

	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id=$3`

	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetObjectRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	if _, err := db.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err := db.Exec(query, _type, datetime, id)
	return err
}

func SetObject(id string, _type string) error {
	if err := SetAttachmentFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewFromDB(id, _type); err != nil {
		return err
	}

	return SetObjectFromDB(id, _type)
}

func SetObjectAndReplies(id string, _type string) error {
	if err := SetAttachmentFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewFromDB(id, _type); err != nil {
		return err
	}

	if err := SetObjectRepliesFromDB(id, _type); err != nil {
		return err
	}

	if err := SetAttachmentRepliesFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewRepliesFromDB(id, _type); err != nil {
		return err
	}

	return SetObjectFromDB(id, _type)
}

func DeleteObject(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := DeletePreviewFromDB(id); err != nil {
		return err
	}

	if err := DeleteObjectFromDB(id); err != nil {
		return err
	}

	return DeleteObjectRepliedTo(id)
}

func TombstoneObject(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := TombstonePreviewFromDB(id); err != nil {
		return err
	}

	return TombstoneObjectFromDB(id)
}

func TombstoneObjectAndReplies(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := TombstonePreviewFromDB(id); err != nil {
		return err
	}

	if err := TombstoneObjectRepliesFromDB(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentRepliesFromDB(id); err != nil {
		return err
	}

	if err := TombstonePreviewRepliesFromDB(id); err != nil {
		return err
	}

	return TombstoneObjectFromDB(id)
}

func GetRandomCaptcha() (string, error) {
	var verify string

	query := `select identifier from verification where type='captcha' order by random() limit 1`

	rows, err := db.Query(query)
	if err != nil {
		return verify, err
	}
	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&verify); err != nil {
		return verify, err
	}

	return verify, nil
}

func GetCaptchaTotal() (int, error) {
	query := `select count(*) from verification where type='captcha'`

	rows, err := db.Query(query)
	if err != nil {
		return 0, err
	}

	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func GetCaptchaCodeDB(verify string) (string, error) {
	query := `select code from verification where identifier=$1 limit 1`

	rows, err := db.Query(query, verify)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var code string

	rows.Next()
	if err := rows.Scan(&code); err != nil {
		fmt.Println("Could not get verification captcha")
	}

	return code, nil
}

func GetActorAuth(actor string) ([]string, error) {
	var auth []string

	query := `select type from actorauth where board=$1`

	rows, err := db.Query(query, actor)
	if err != nil {
		return auth, err
	}
	defer rows.Close()

	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return auth, err
		}

		auth = append(auth, e)
	}

	return auth, nil
}

func DeleteCaptchaCodeDB(verify string) error {
	query := `delete from verification where identifier=$1`

	_, err := db.Exec(query, verify)
	if err != nil {
		return err
	}

	return os.Remove("./" + verify)
}

func GetActorReportedTotal(id string) (int, error) {
	query := `select count(id) from reported where board=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return 0, err
	}

	defer rows.Close()

	var count int
	for rows.Next() {
		rows.Scan(&count)
	}

	return count, nil
}

func GetActorReportedDB(id string) ([]activitypub.ObjectBase, error) {
	var nObj []activitypub.ObjectBase

	query := `select id, count, reason from reported where board=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return nObj, err
	}

	defer rows.Close()

	for rows.Next() {
		var obj activitypub.ObjectBase

		rows.Scan(&obj.Id, &obj.Size, &obj.Content)

		nObj = append(nObj, obj)
	}

	return nObj, nil
}

func MarkObjectSensitive(id string, sensitive bool) error {
	var query = `update activitystream set sensitive=$1 where id=$2`
	if _, err := db.Exec(query, sensitive, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set sensitive=$1 where id=$2`
	_, err := db.Exec(query, sensitive, id)
	return err
}

//if limit less than 1 return all news items
func GetNewsFromDB(limit int) ([]NewsItem, error) {
	var news []NewsItem

	var query string
	if limit > 0 {
		query = `select title, content, time from newsItem order by time desc limit $1`
	} else {
		query = `select title, content, time from newsItem order by time desc`
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = db.Query(query, limit)
	} else {
		rows, err = db.Query(query)
	}

	if err != nil {
		return news, nil
	}

	defer rows.Close()
	for rows.Next() {
		n := NewsItem{}
		var content string
		if err := rows.Scan(&n.Title, &content, &n.Time); err != nil {
			return news, err
		}

		content = strings.ReplaceAll(content, "\n", "<br>")
		n.Content = template.HTML(content)

		news = append(news, n)
	}

	return news, nil
}

func GetNewsItemFromDB(timestamp int) (NewsItem, error) {
	var news NewsItem
	var content string
	query := `select title, content, time from newsItem where time=$1 limit 1`

	rows, err := db.Query(query, timestamp)
	if err != nil {
		return news, err
	}

	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&news.Title, &content, &news.Time); err != nil {
		return news, err
	}

	content = strings.ReplaceAll(content, "\n", "<br>")
	news.Content = template.HTML(content)

	return news, nil
}

func deleteNewsItemFromDB(timestamp int) error {
	query := `delete from newsItem where time=$1`
	_, err := db.Exec(query, timestamp)
	return err
}

func WriteNewsToDB(news NewsItem) error {
	query := `insert into newsItem (title, content, time) values ($1, $2, $3)`

	_, err := db.Exec(query, news.Title, news.Content, time.Now().Unix())
	return err
}

func GetActorAutoSubscribeDB(id string) (bool, error) {
	query := `select autosubscribe from actor where id=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return false, err
	}

	var subscribed bool
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&subscribed)
	return subscribed, err
}

func SetActorAutoSubscribeDB(id string) error {
	current, err := GetActorAutoSubscribeDB(id)
	if err != nil {
		return err
	}

	query := `update actor set autosubscribe=$1 where id=$2`

	_, err = db.Exec(query, !current, id)
	return err
}

func AddInstanceToInactiveDB(instance string) error {
	query := `select timestamp from inactive where instance=$1`

	rows, err := db.Query(query, instance)
	if err != nil {
		return err
	}

	var timeStamp string
	defer rows.Close()
	rows.Next()
	rows.Scan(&timeStamp)

	if timeStamp == "" {
		query := `insert into inactive (instance, timestamp) values ($1, $2)`

		_, err := db.Exec(query, instance, time.Now().UTC().Format(time.RFC3339))
		return err
	}

	if !IsInactiveTimestamp(timeStamp) {
		return nil
	}

	query = `delete from following where following like $1`
	if _, err := db.Exec(query, "%"+instance+"%"); err != nil {
		return err
	}

	query = `delete from follower where follower like $1`
	if _, err = db.Exec(query, "%"+instance+"%"); err != nil {
		return err
	}

	return DeleteInstanceFromInactiveDB(instance)
}

func DeleteInstanceFromInactiveDB(instance string) error {
	query := `delete from inactive where instance=$1`

	_, err := db.Exec(query, instance)
	return err
}

func IsInactiveTimestamp(timeStamp string) bool {
	stamp, _ := time.Parse(time.RFC3339, timeStamp)
	if time.Now().UTC().Sub(stamp).Hours() > 48 {
		return true
	}

	return false
}

func ArchivePosts(actor activitypub.Actor) error {
	if actor.Id != "" && actor.Id != config.Domain {
		col, err := GetAllActorArchiveDB(actor.Id, 165)
		if err != nil {
			return err
		}

		for _, e := range col.OrderedItems {
			for _, k := range e.Replies.OrderedItems {
				if err := UpdateObjectTypeDB(k.Id, "Archive"); err != nil {
					return err
				}
			}

			if err := UpdateObjectTypeDB(e.Id, "Archive"); err != nil {
				return err
			}
		}
	}

	return nil
}

func GetAllActorArchiveDB(id string, offset int) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`

	rows, err := db.Query(query, id, offset)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var post activitypub.ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, err
		}

		post.Replies, _, _, err = GetObjectRepliesDB(post)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorCollectionDBType(actorId string, nType string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc`

	rows, err := db.Query(query, actorId, nType)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies activitypub.CollectionBase

		post.Replies = &replies

		var err error
		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorCollectionDBTypeLimit(actorId string, nType string, limit int) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc limit $3`

	rows, err := db.Query(query, actorId, nType, limit)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post activitypub.ObjectBase
		var actor activitypub.Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies activitypub.CollectionBase

		post.Replies = &replies

		var err error
		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func UpdateObjectTypeDB(id string, nType string) error {
	query := `update activitystream set type=$2 where id=$1 and type !='Tombstone'`
	if _, err := db.Exec(query, id, nType); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$2 where id=$1 and type !='Tombstone'`
	_, err := db.Exec(query, id, nType)
	return err
}

func UnArchiveLast(actorId string) error {
	col, err := GetActorCollectionDBTypeLimit(actorId, "Archive", 1)
	if err != nil {
		return err
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := UpdateObjectTypeDB(k.Id, "Note"); err != nil {
				return err
			}
		}

		if err := UpdateObjectTypeDB(e.Id, "Note"); err != nil {
			return err
		}
	}

	return nil
}

func SetObjectType(id string, nType string) error {
	col, err := GetObjectFromDB(id)
	if err != nil {
		return err
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := UpdateObjectTypeDB(k.Id, nType); err != nil {
				return err
			}
		}

		if err := UpdateObjectTypeDB(e.Id, nType); err != nil {
			return err
		}
	}

	return nil
}

func GetObjectTypeDB(id string) (string, error) {
	query := `select type from activitystream where id=$1 union select type from cacheactivitystream where id=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var nType string
	rows.Next()
	rows.Scan(&nType)

	return nType, nil
}

func IsReplyInThread(inReplyTo string, id string) (bool, error) {
	obj, _, err := webfinger.CheckValidActivity(inReplyTo)
	if err != nil {
		return false, err
	}

	for _, e := range obj.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true, nil
		}
	}

	return false, nil
}

func GetActorsFollowPostFromId(actors []string, id string) (activitypub.Collection, error) {
	var collection activitypub.Collection

	for _, e := range actors {
		tempCol, err := GetObjectByIDFromDB(e + "/" + id)
		if err != nil {
			return collection, err
		}

		if len(tempCol.OrderedItems) > 0 {
			collection = tempCol
			return collection, nil
		}
	}

	return collection, nil
}

func IsReplyToOP(op string, link string) (string, bool, error) {
	if op == link {
		return link, true, nil
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
	if err != nil {
		return op, false, err
	}
	defer rows.Close()

	var id string
	rows.Next()
	if err := rows.Scan(&id); err != nil {
		return id, false, err
	}

	if id != "" {
		return id, true, nil
	}

	return "", false, nil
}

func GetReplyOP(link string) (string, error) {
	query := `select id from replies where id in (select inreplyto from replies where id=$1) and inreplyto=''`

	rows, err := db.Query(query, link)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var id string

	rows.Next()
	err = rows.Scan(&id)
	return id, err
}

func StartupArchive() error {
	for _, e := range FollowingBoards {
		actor, err := GetActorFromDB(e.Id)
		if err != nil {
			return err
		}

		if err := ArchivePosts(actor); err != nil {
			return err
		}
	}

	return nil
}

func CheckInactive() {
	for true {
		CheckInactiveInstances()
		time.Sleep(24 * time.Hour)
	}
}

func CheckInactiveInstances() (map[string]string, error) {
	instances := make(map[string]string)
	query := `select following from following`

	rows, err := db.Query(query)
	if err != nil {
		return instances, err
	}
	defer rows.Close()

	for rows.Next() {
		var instance string
		if err := rows.Scan(&instance); err != nil {
			return instances, err
		}

		instances[instance] = instance
	}

	query = `select follower from follower`
	rows, err = db.Query(query)
	if err != nil {
		return instances, err
	}
	defer rows.Close()

	for rows.Next() {
		var instance string
		if err := rows.Scan(&instance); err != nil {
			return instances, err
		}

		instances[instance] = instance
	}

	re := regexp.MustCompile(config.Domain + `(.+)?`)
	for _, e := range instances {
		actor, err := webfinger.GetActor(e)
		if err != nil {
			return instances, err
		}

		if actor.Id == "" && !re.MatchString(e) {
			if err := AddInstanceToInactiveDB(e); err != nil {
				return instances, err
			}
		} else {
			if err := DeleteInstanceFromInactiveDB(e); err != nil {
				return instances, err
			}
		}
	}

	return instances, nil
}

func GetAdminAuth() (string, string, error) {
	query := fmt.Sprintf("select identifier, code from boardaccess where board='%s' and type='admin'", config.Domain)

	rows, err := db.Query(query)
	if err != nil {
		return "", "", err
	}

	var code string
	var identifier string

	rows.Next()
	err = rows.Scan(&identifier, &code)

	return code, identifier, err
}

func UpdateObjectWithPreview(id string, preview string) error {
	query := `update activitystream set preview=$1 where attachment=$2`

	_, err := db.Exec(query, preview, id)
	return err
}

func GetObjectsWithoutPreviewsCallback(callback func(id string, href string, mediatype string, name string, size int, published time.Time) error) error {
	query := `select id, href, mediatype, name, size, published from activitystream where id in (select attachment from activitystream where attachment!='' and preview='')`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var href string
		var mediatype string
		var name string
		var size int
		var published time.Time

		if err := rows.Scan(&id, &href, &mediatype, &name, &size, &published); err != nil {
			return err
		}

		if err := callback(id, href, mediatype, name, size, published); err != nil {
			return err
		}
	}

	return nil
}

func AddFollower(id string, follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`

	_, err := db.Exec(query, id, follower)
	return err
}

func IsHashBanned(hash string) (bool, error) {
	var h string

	query := `select hash from bannedmedia where hash=$1`

	rows, err := db.Query(query, hash)
	if err != nil {
		return true, err
	}
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&h)

	return h == hash, err
}
