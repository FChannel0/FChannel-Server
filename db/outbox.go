package db

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
)

func GetCollectionFromPath(path string) (activitypub.Collection, error) {
	var nColl activitypub.Collection
	var result []activitypub.ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var actor activitypub.Actor
		var post activitypub.ObjectBase
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		post.InReplyTo, err = GetInReplyToDB(post)
		if err != nil {
			return nColl, err
		}

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt

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

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromPath(path string) (activitypub.ObjectBase, error) {
	var nObj activitypub.ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := db.Query(query, path)
	if err != nil {
		return nObj, err
	}

	defer rows.Close()
	rows.Next()
	var attachID string
	var previewID string

	var nActor activitypub.Actor
	nObj.Actor = nActor.Id

	if err := rows.Scan(&nObj.Id, &nObj.Name, &nObj.Content, &nObj.Type, &nObj.Published, &nObj.AttributedTo, &attachID, &previewID, &nObj.Actor); err != nil {
		return nObj, err
	}

	var postCnt int
	var imgCnt int

	nObj.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(nObj)
	if err != nil {
		return nObj, err
	}

	nObj.Replies.TotalItems, nObj.Replies.TotalImgs, err = GetObjectRepliesCount(nObj)
	if err != nil {
		return nObj, err
	}

	nObj.Replies.TotalItems = nObj.Replies.TotalItems + postCnt
	nObj.Replies.TotalImgs = nObj.Replies.TotalImgs + imgCnt

	nObj.Attachment, err = GetObjectAttachment(attachID)
	if err != nil {
		return nObj, err
	}

	nObj.Preview, err = GetObjectPreview(previewID)
	return nObj, err
}
