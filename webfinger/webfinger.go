package webfinger

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

type Webfinger struct {
	Subject string          `json:"subject,omitempty"`
	Links   []WebfingerLink `json:"links,omitempty"`
}

type WebfingerLink struct {
	Rel  string `json:"rel,omitempty"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

var ActorCache = make(map[string]activitypub.Actor)

func GetActor(id string) (activitypub.Actor, error) {
	var respActor activitypub.Actor

	if id == "" {
		return respActor, nil
	}

	actor, instance := activitypub.GetActorInstance(id)

	if ActorCache[actor+"@"+instance].Id != "" {
		respActor = ActorCache[actor+"@"+instance]
		return respActor, nil
	}

	req, err := http.NewRequest("GET", strings.TrimSpace(id), nil)
	if err != nil {
		return respActor, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)

	if err != nil {
		return respActor, err
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &respActor); err != nil {
		return respActor, err
	}

	ActorCache[actor+"@"+instance] = respActor

	return respActor, nil
}

//looks for actor with pattern of board@instance
func FingerActor(path string) (activitypub.Actor, error) {
	var nActor activitypub.Actor

	actor, instance := activitypub.GetActorInstance(path)

	if actor == "" && instance == "" {
		return nActor, nil
	}

	if ActorCache[actor+"@"+instance].Id != "" {
		nActor = ActorCache[actor+"@"+instance]
		return nActor, nil
	}

	r, err := FingerRequest(actor, instance)
	if err != nil {
		return nActor, err
	}

	if r != nil && r.StatusCode == 200 {
		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)

		if err := json.Unmarshal(body, &nActor); err != nil {
			return nActor, err
		}

		ActorCache[actor+"@"+instance] = nActor
	}

	// TODO: this just falls through and returns a blank Actor object. do something?
	return nActor, nil
}

func FingerRequest(actor string, instance string) (*http.Response, error) {
	acct := "acct:" + actor + "@" + instance

	// TODO: respect https
	req, err := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)
	if err != nil {
		return nil, err
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return resp, err
	}

	var finger Webfinger

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if err := json.Unmarshal(body, &finger); err != nil {
			return resp, err
		}
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, err := http.NewRequest("GET", e.Href, nil)
				if err != nil {
					return resp, err
				}

				req.Header.Set("Accept", config.ActivityStreams)

				resp, err := util.RouteProxy(req)
				return resp, err
			}
		}
	}

	return resp, nil
}

func CheckValidActivity(id string) (activitypub.Collection, bool, error) {
	var respCollection activitypub.Collection

	re := regexp.MustCompile(`.+\.onion(.+)?`)
	if re.MatchString(id) {
		id = strings.Replace(id, "https", "http", 1)
	}

	req, err := http.NewRequest("GET", id, nil)
	if err != nil {
		return respCollection, false, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return respCollection, false, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &respCollection); err != nil {
		return respCollection, false, err
	}

	if respCollection.AtContext.Context == "https://www.w3.org/ns/activitystreams" && respCollection.OrderedItems[0].Id != "" {
		return respCollection, true, nil
	}

	return respCollection, false, nil
}
