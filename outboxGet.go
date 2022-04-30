package main

import (
	"net/http"

	"encoding/json"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func GetActorOutbox(w http.ResponseWriter, r *http.Request) error {
	actor, err := webfinger.GetActorFromPath(r.URL.Path, "/")
	if err != nil {
		return err
	}

	var collection activitypub.Collection

	c, err := activitypub.GetActorObjectCollectionFromDB(actor.Id)
	if err != nil {
		return err
	}
	collection.OrderedItems = c.OrderedItems

	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, err = activitypub.GetObjectPostsTotalDB(actor)
	if err != nil {
		return err
	}

	collection.TotalImgs, err = activitypub.GetObjectImgsTotalDB(actor)
	if err != nil {
		return err
	}

	enc, _ := json.Marshal(collection)

	w.Header().Set("Content-Type", config.ActivityStreams)
	_, err = w.Write(enc)
	return err
}
