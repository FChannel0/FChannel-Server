package webfinger

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

// TODO: All of these functions in this file I don't know where to place so they'll remain here until I find a better place for them.

func GetActorCollection(collection string) (activitypub.Collection, error) {
	var nCollection activitypub.Collection

	if collection == "" {
		return nCollection, errors.New("invalid collection")
	}

	req, err := http.NewRequest("GET", collection, nil)
	if err != nil {
		return nCollection, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return nCollection, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			if err := json.Unmarshal(body, &nCollection); err != nil {
				return nCollection, err
			}
		}
	}

	return nCollection, nil
}

func GetCollectionFromReq(path string) (activitypub.Collection, error) {
	var respCollection activitypub.Collection

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return respCollection, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return respCollection, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &respCollection)
	return respCollection, err
}
