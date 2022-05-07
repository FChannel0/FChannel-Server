package db

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func WriteObjectReplyToCache(obj activitypub.ObjectBase) error {
	for i, e := range obj.InReplyTo {
		res, err := IsReplyInThread(obj.InReplyTo[0].Id, e.Id)
		if err != nil {
			return err
		}

		if i == 0 || res {
			query := `select id from replies where id=$1`

			rows, err := config.DB.Query(query, obj.Id)
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

			_, err = config.DB.Exec(query, obj.Id, e.Id)
			if err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `insert into cachereplies (id, inreplyto) values ($1, $2)`

		_, err := config.DB.Exec(query, obj.Id, "")
		return err
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
		if _, err := e.WriteActorObjectToCache(); err != nil {
			return err
		}
	}

	return nil
}
