package main

import (
	"database/sql"
	"net/http"

	"encoding/json"

	"github.com/FChannel0/FChannel-Server/activitypub"
	_ "github.com/lib/pq"
	"golang.org/x/perf/storage/db"
)

func GetActorOutbox(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	actor := GetActorFromPath(r.URL.Path, "/")
	var collection activitypub.Collection

	collection.OrderedItems = GetActorObjectCollectionFromDB(actor.Id).OrderedItems
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems = GetObjectPostsTotalDB(actor)
	collection.TotalImgs = GetObjectImgsTotalDB(actor)

	enc, _ := json.Marshal(collection)

	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}

func GetCollectionFromPath(path string) activitypub.Collection {

	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)

	CheckError(err, "error query collection path from db")

	defer rows.Close()

	for rows.Next() {
		var actor activitypub.Actor
		var post activitypub.ObjectBase
		var attachID string
		var previewID string

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)

		CheckError(err, "error scan object into post struct from path")

		post.Actor = actor.Id

		post.InReplyTo = GetInReplyToDB(post)

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCount(post)

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt

		post.Attachment = GetObjectAttachment(attachID)

		post.Preview = GetObjectPreview(previewID)

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl
}

func GetObjectFromPath(path string) activitypub.ObjectBase {
	var nObj activitypub.ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)

	CheckError(err, "error query collection path from db")

	defer rows.Close()
	rows.Next()
	var attachID string
	var previewID string

	var nActor activitypub.Actor
	nObj.Actor = nActor.Id

	err = rows.Scan(&nObj.Id, &nObj.Name, &nObj.Content, &nObj.Type, &nObj.Published, &nObj.AttributedTo, &attachID, &previewID, &nObj.Actor)

	CheckError(err, "error scan object into post struct from path")

	var postCnt int
	var imgCnt int

	nObj.Replies, postCnt, imgCnt = GetObjectRepliesDB(nObj)

	nObj.Replies.TotalItems, nObj.Replies.TotalImgs = GetObjectRepliesCount(nObj)

	nObj.Replies.TotalItems = nObj.Replies.TotalItems + postCnt
	nObj.Replies.TotalImgs = nObj.Replies.TotalImgs + imgCnt

	nObj.Attachment = GetObjectAttachment(attachID)

	nObj.Preview = GetObjectPreview(previewID)

	return nObj
}
