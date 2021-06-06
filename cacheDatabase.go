package main

import "fmt"
import "database/sql"
import _ "github.com/lib/pq"

func WriteObjectToCache(db *sql.DB, obj ObjectBase) ObjectBase {
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			WritePreviewToCache(db, *obj.Preview)
		}
		
		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(db, obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(db, obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(db, obj)
	}

	WriteObjectReplyToDB(db, obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteObjectToCache(db, e)
		}
	}

	return obj
}

func WriteActorObjectToCache(db *sql.DB, obj ObjectBase) ObjectBase {
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			WritePreviewToCache(db, *obj.Preview)
		}
		
		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(db, obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(db, obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(db, obj)
	}

	WriteActorObjectReplyToDB(db, obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteActorObjectToCache(db, e)
		}
	}

	return obj
}

func WriteActivitytoCache(db *sql.DB, obj ObjectBase) {

	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting  obj id from cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}		

	query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id, obj.TripCode)	
	
	if e != nil{
		fmt.Println("error inserting new activity cache")
		panic(e)			
	}	
}

func WriteActivitytoCacheWithAttachment(db *sql.DB, obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) {
	
	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting activity with attachment obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}		

	query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id, obj.TripCode)	
	
	if e != nil{
		fmt.Println("error inserting new activity with attachment cache")
		panic(e)			
	}	
}

func WriteAttachmentToCache(db *sql.DB, obj ObjectBase) {

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting attachment obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}
	
	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Published, obj.AttributedTo, obj.MediaType, obj.Size)	
	
	if e != nil{
		fmt.Println("error inserting new attachment cache")
		panic(e)			
	}
}

func WritePreviewToCache(db *sql.DB, obj NestedObjectBase) {

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting preview obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}	
	
	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Published, obj.AttributedTo, obj.MediaType, obj.Size)
	
	if e != nil{
		fmt.Println("error inserting new preview cache")
		panic(e)			
	}
}

func WriteObjectReplyToCache(db *sql.DB, obj ObjectBase) {
	
	for i, e := range obj.InReplyTo {
		if(i == 0 || IsReplyInThread(db, obj.InReplyTo[0].Id, e.Id)){

			query := `select id from replies where id=$1`

			rows, err := db.Query(query, obj.Id)

			CheckError(err, "error selecting obj id cache reply")

			var id string 
			defer rows.Close()
			rows.Next()
			rows.Scan(&id)

			if id != "" {
				return
			}
			
			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			_, err = db.Exec(query, obj.Id, e.Id)			
			
			if err != nil{
				fmt.Println("error inserting replies cache")
				panic(err)			
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `insert into cachereplies (id, inreplyto) values ($1, $2)`

		_, err := db.Exec(query, obj.Id, "")			
		
		if err != nil{
			fmt.Println("error inserting replies cache")
			panic(err)			
		}
	}	
}

func WriteObjectReplyCache(db *sql.DB, obj ObjectBase) {

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {

			query := `select inreplyto from cachereplies where id=$1`

			rows, err := db.Query(query, obj.Id)

			CheckError(err, "error selecting obj id cache reply")

			var inreplyto string 		
			defer rows.Close()
			rows.Next()
			rows.Scan(&inreplyto)

			if inreplyto != "" {
				return
			}
			
			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			_, err = db.Exec(query, e.Id, obj.Id)			
			
			if err != nil{
				fmt.Println("error inserting replies cache")
				panic(err)			
			}

			if !IsObjectLocal(db, e.Id) {
				WriteObjectToCache(db, e)
			}

		}
		return 
	}
}

func WriteActorToCache(db *sql.DB, actorID string) {
	actor := GetActor(actorID)
	collection := GetActorCollection(actor.Outbox)

	for _, e := range collection.OrderedItems {
		WriteActorObjectToCache(db, e)
	}
}

func DeleteActorCache(db *sql.DB, actorID string) { 
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1)`

	rows, err := db.Query(query, actorID)

	CheckError(err, "error selecting actors activity from cache")

	defer rows.Close()

	for rows.Next() {
		var id string
		rows.Scan(&id)

		DeleteObject(db, id)
	}
}
