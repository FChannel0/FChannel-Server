package main

import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"

func GetActorOutbox(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	actor := GetActorFromPath(db, r.URL.Path, "/")
	var collection Collection

	collection.OrderedItems = GetObjectFromDB(db, actor).OrderedItems

	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor
	collection.Actor.AtContext.Context = ""	

	collection.TotalItems = GetObjectPostsTotalDB(db, actor)
	collection.TotalImgs = GetObjectImgsTotalDB(db, actor)

	enc, _ := json.MarshalIndent(collection, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetObjectsFromFollow(db *sql.DB, actor Actor) []ObjectBase {
	var followingCol Collection
	var followObj []ObjectBase
	followingCol = GetActorCollection(actor.Following)
	for _, e := range followingCol.Items {
		var followOutbox Collection
		followOutbox = GetObjectFromCache(db, e.Id)
		for _, e := range followOutbox.OrderedItems {
			followObj = append(followObj, e)
		}
	}
	return followObj
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

		post.Actor = &actor

		post.InReplyTo = GetInReplyToDB(db, post)

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

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
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)

	CheckError(err, "error query collection path from db")
	
	defer rows.Close()

	for rows.Next(){
		var post ObjectBase
		var attachID string
		var previewID string

		var nActor Actor
		post.Actor = &nActor
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &post.Actor.Id)
		
		CheckError(err, "error scan object into post struct from path")


		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt		

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nObj = result[0]

	return nObj		
}
