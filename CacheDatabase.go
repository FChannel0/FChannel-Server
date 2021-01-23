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

	WriteObjectReplyToCache(db, obj)	

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

	query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor) values ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id)	
	
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

	query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id)	
	
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

func GetActivityFromCache(db *sql.DB, id string) Collection {
	var nColl Collection
	var nActor Actor
	var result []ObjectBase

	nColl.Actor = &nActor

	query := `select  actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor from  activitystream where id=$1 order by updated asc`

	rows, err := db.Query(query, id)	

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string	
		var previewID	string
		
		
		err = rows.Scan(&nColl.Actor.Id, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = &actor

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt = GetObjectRepliesCache(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCacheCount(db, post)

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt		

		post.Attachment = GetObjectAttachmentCache(db, attachID)

		post.Preview = GetObjectPreviewCache(db, previewID)

		result = append(result, post)		
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetObjectFromCache(db *sql.DB, id string) Collection {
	var nColl Collection
	var result []ObjectBase


	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor from activitystream where actor=$1 and id in (select id from cachereplies where inreplyto='') and type='Note' order by updated asc`	

	rows, err := db.Query(query, id)	

	CheckError(err, "error query object from db cache")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id)
		
		CheckError(err, "error scan object into post struct cache")

		post.Actor = &actor

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesCache(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCacheCount(db, post)

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt		

		post.Attachment = GetObjectAttachmentCache(db, attachID)

		post.Preview = GetObjectPreviewCache(db, previewID)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func WriteObjectReplyToCache(db *sql.DB, obj ObjectBase) {
	for i, e := range obj.InReplyTo {
		if(i == 0 || IsReplyInThread(db, obj.InReplyTo[0].Id, e.Id)){

			query := `select id from cachereplies where id=$1`

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
}

func GetObjectRepliesCache(db *sql.DB, parent ObjectBase) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from cacheactivitystream WHERE id in (select id from cachereplies where inreplyto=$1) and type='Note' order by published asc`

	rows, err := db.Query(query, parent.Id)	

	CheckError(err, "error with replies db query")	

	defer rows.Close()	
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)


		CheckError(err, "error with replies db scan")

		post.Actor = &actor

		var postCnt int
		var imgCnt int		
		post.Replies, postCnt, imgCnt = GetObjectRepliesRepliesCache(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesCacheCount(db, post)
		
		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt		
		
		post.Attachment = GetObjectAttachmentCache(db, attachID)

		post.Preview = GetObjectPreviewCache(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result

	remoteCollection := GetObjectRepliesRemote(db, parent)

	var postc int
	var imgc int
	for _, e := range remoteCollection.OrderedItems {
		nColl.OrderedItems = append(nColl.OrderedItems, e)
		postc = postc + 1
		if len(e.Attachment) > 0 {
			imgc = imgc + 1
		}
	}

	return &nColl, postc, imgc
}

func GetObjectRepliesRepliesCache(db *sql.DB, parent ObjectBase) (*CollectionBase, int, int) {

	var nColl CollectionBase
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from cacheactivitystream where id in (select id from cachereplies where inreplyto=$1) and type='Note' order by published asc`

	rows, err := db.Query(query, parent.Id)	

	CheckError(err, "error with replies replies cache query")

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)


		CheckError(err, "error with replies replies cache scan")

		post.Actor = &actor

		post.Attachment = GetObjectAttachmentCache(db, attachID)

		post.Preview = GetObjectPreviewCache(db, previewID)				

		result = append(result, post)			
	}

	remoteCollection := GetObjectRepliesRemote(db, parent)

	var postc int
	var imgc int
	for _, e := range remoteCollection.OrderedItems {
		
		nColl.OrderedItems = append(nColl.OrderedItems, e)
		postc = postc + 1
		if len(e.Attachment) > 0 {
			imgc = imgc + 1
		}			
	}	

	nColl.OrderedItems = result

	return &nColl, postc, imgc
}

func GetObjectRepliesCacheCount(db *sql.DB, parent ObjectBase) (int, int) {

	var countId int
	var countImg int 

	query := `select count(id) from replies where inreplyto=$1 and id in (select id from activitystream where type='Note')`
	
	rows, err := db.Query(query, parent.Id)	
	
	CheckError(err, "error with replies count db query")

	defer rows.Close()
	rows.Next()
	rows.Scan(&countId)

	query = `select count(attachment) from activitystream where id in (select id from replies where inreplyto=$1) and attachment != ''`
	
	rows, err = db.Query(query,  parent.Id)	

	CheckError(err, "error with select attachment count db query")
	
	defer rows.Close()	
	rows.Next()
	rows.Scan(&countImg)

	return countId, countImg
}

func GetObjectAttachmentCache(db *sql.DB, id string) []ObjectBase {

	var attachments []ObjectBase	

	query := `select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1`

	rows, err := db.Query(query,  id)	

	CheckError(err, "could not select object attachment query")

	defer rows.Close()
	for rows.Next() {
		var attachment = new(ObjectBase)

		err = rows.Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published)
		if err !=nil{
			fmt.Println("error with attachment db query")
			panic(err)
		}

		attachments = append(attachments, *attachment)
	}

	return attachments
}

func GetObjectPreviewCache(db *sql.DB, id string) *NestedObjectBase {

	var preview NestedObjectBase

	query := `select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1`

	rows, err := db.Query(query, id)	

	CheckError(err, "could not select object preview query")	

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published)
	}

	return &preview
}

func DeleteObjectFromCache(db *sql.DB, id string) {
	query := `select attachment, preview from cacheactivitystream where id=$1 `

	rows, err := db.Query(query, id)
	CheckError(err, "could not select cache activitystream")

	var attachment string
	var preview string
	
	defer rows.Close()	
	rows.Next()
	rows.Scan(&attachment, &preview)

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, attachment)
	CheckError(err, "could not delete attachmet cache activitystream")

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, preview)
	CheckError(err, "could not delete preview cache activitystream")

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, id)
	CheckError(err, "could not delete object cache activitystream")

	query = `delete from cachereplies where id=$1`
	_, err = db.Exec(query, id)
	CheckError(err, "could not delete  cache replies activitystream")
}

func GetObjectPostsTotalCache(db *sql.DB, actor Actor) int{

	count := 0
	query := `select count(id) from cacheactivitystream where actor=$1 and id in (select id from cachereplies where inreplyto='' and type='Note')`

	rows, err := db.Query(query, actor.Id)	

	CheckError(err, "could not select post total count query")		

	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&count)
		CheckError(err, "error with total post db scan")
	}
	
	return count
}

func GetObjectImgsTotalCache(db *sql.DB, actor Actor) int{

	count := 0
	query := `select count(attachment) from cacheactivitystream where actor=$1 and id in (select id from cachereplies where inreplyto='' and type='Note' )`

	rows, err := db.Query(query, actor.Id)	

	CheckError(err, "error with posts total db query")			

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&count)

		CheckError(err, "error with total post db scan")
	}
	
	return count
}
