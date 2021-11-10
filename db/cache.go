package db

import (
	"fmt"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func WriteObjectToCache(obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	if res, err := IsPostBlacklist(obj.Content); err == nil && res {
		fmt.Println("\n\nBlacklist post blocked\n\n")
		return obj, nil
	} else {
		return obj, err
	}

	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			WritePreviewToCache(*obj.Preview)
		}

		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(obj)
	}

	WriteObjectReplyToDB(obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteObjectToCache(e)
		}
	}

	return obj, nil
}

func WriteActorObjectToCache(obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	if res, err := IsPostBlacklist(obj.Content); err == nil && res {
		fmt.Println("\n\nBlacklist post blocked\n\n")
		return obj, nil
	} else if err != nil {
		return obj, err
	}

	if len(obj.Attachment) > 0 {
		if res, err := IsIDLocal(obj.Id); err == nil && res {
			return obj, err
		} else if err != nil {
			return obj, err
		}

		if obj.Preview.Href != "" {
			WritePreviewToCache(*obj.Preview)
		}

		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(obj)
	}

	WriteActorObjectReplyToDB(obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteActorObjectToCache(e)
		}
	}

	return obj, nil
}

func WriteActivitytoCache(obj activitypub.ObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteActivitytoCacheWithAttachment(obj activitypub.ObjectBase, attachment activitypub.ObjectBase, preview activitypub.NestedObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteAttachmentToCache(obj activitypub.ObjectBase) error {
	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}

func WritePreviewToCache(obj activitypub.NestedObjectBase) error {
	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}

func WriteObjectReplyToCache(obj activitypub.ObjectBase) error {
	for i, e := range obj.InReplyTo {
		res, err := IsReplyInThread(obj.InReplyTo[0].Id, e.Id)
		if err != nil {
			return err
		}

		if i == 0 || res {
			query := `select id from replies where id=$1`

			rows, err := db.Query(query, obj.Id)
			if err != nil {
				return err
			}
			defer rows.Close()

			var id string
			rows.Next()
			err = rows.Scan(&id)
			if err != nil {
				return err
			} else if id != "" {
				return nil // TODO: error?
			}

			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			_, err = db.Exec(query, obj.Id, e.Id)
			if err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `insert into cachereplies (id, inreplyto) values ($1, $2)`

		_, err := db.Exec(query, obj.Id, "")
		return err
	}

	return nil
}

func WriteObjectReplyCache(obj activitypub.ObjectBase) error {
	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {

			query := `select inreplyto from cachereplies where id=$1`

			rows, err := db.Query(query, obj.Id)
			if err != nil {
				return err
			}
			defer rows.Close()

			var inreplyto string
			rows.Next()
			err = rows.Scan(&inreplyto)
			if err != nil {
				return err
			} else if inreplyto != "" {
				return nil // TODO: error?
			}

			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			if _, err := db.Exec(query, e.Id, obj.Id); err != nil {
				return err
			}

			if res, err := IsObjectLocal(e.Id); err == nil && !res {
				if _, err := WriteObjectToCache(e); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

		}
	}

	return nil
}

func WriteActorToCache(actorID string) error {
	actor, err := webfinger.FingerActor(actorID)
	if err != nil {
		return err
	}

	collection, err := webfinger.GetActorCollection(actor.Outbox)
	if err != nil {
		return err
	}

	for _, e := range collection.OrderedItems {
		if _, err := WriteActorObjectToCache(e); err != nil {
			return err
		}
	}

	return nil
}

func DeleteActorCache(actorID string) error {
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1)`

	rows, err := db.Query(query, actorID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}

		if err := DeleteObject(id); err != nil {
			return err
		}
	}

	return nil
}
