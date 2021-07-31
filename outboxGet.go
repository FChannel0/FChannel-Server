package main

import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"

func GetActorOutbox(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	actor := GetActorFromPath(db, r.URL.Path, "/")
	var collection Collection

	collection.OrderedItems = GetActorObjectCollectionFromDB(db, actor.Id).OrderedItems
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems = GetObjectPostsTotalDB(db, actor)
	collection.TotalImgs = GetObjectImgsTotalDB(db, actor)

	enc, _ := json.Marshal(collection)

	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}

func GetCollectionFromPath(db *sql.DB, path string) Collection {

	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)

	CheckError(err, "error query collection path from db")

	defer rows.Close()

	for rows.Next(){
		var actor Actor
		var post ObjectBase
		var attachID string
		var previewID string

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)

		CheckError(err, "error scan object into post struct from path")

		post.Actor = actor.Id

		post.InReplyTo = GetInReplyToDB(db, post)

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCount(db, post)

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl
}

func GetObjectFromPath(db *sql.DB, path string) ObjectBase{

	var nObj ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)

	CheckError(err, "error query collection path from db")

	defer rows.Close()
	rows.Next()
	var attachID string
	var previewID string

	var nActor Actor
	nObj.Actor = nActor.Id

	err = rows.Scan(&nObj.Id, &nObj.Name, &nObj.Content, &nObj.Type, &nObj.Published, &nObj.AttributedTo, &attachID, &previewID, &nObj.Actor)

	CheckError(err, "error scan object into post struct from path")

	var postCnt int
	var imgCnt int

	nObj.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, nObj)

	nObj.Replies.TotalItems, nObj.Replies.TotalImgs = GetObjectRepliesCount(db, nObj)

	nObj.Replies.TotalItems = nObj.Replies.TotalItems + postCnt
	nObj.Replies.TotalImgs = nObj.Replies.TotalImgs + imgCnt

	nObj.Attachment = GetObjectAttachment(db, attachID)

	nObj.Preview = GetObjectPreview(db, previewID)

	return nObj
}
