package webfinger

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

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
	} else {
		r, _ := FingerRequest(actor, instance)

		if r != nil && r.StatusCode == 200 {
			defer r.Body.Close()

			body, _ := ioutil.ReadAll(r.Body)

			json.Unmarshal(body, &nActor)
			// if err := json.Unmarshal(body, &nActor); err != nil {
			//	return nActor, err
			// }

			ActorCache[actor+"@"+instance] = nActor
		}
	}

	return nActor, nil
}

func FingerRequest(actor string, instance string) (*http.Response, error) {
	acct := "acct:" + actor + "@" + instance

	// TODO: respect https
	req, _ := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)
	// if err != nil {
	//	return nil, err
	// }

	resp, err := util.RouteProxy(req)
	if err != nil {
		return resp, nil
	}

	var finger Webfinger

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		json.Unmarshal(body, &finger)
		// if err := json.Unmarshal(body, &finger); err != nil {
		//	return resp, err
		// }
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, _ := http.NewRequest("GET", e.Href, nil)
				// if err != nil {
				//	return resp, err
				// }

				req.Header.Set("Accept", config.ActivityStreams)

				resp, _ := util.RouteProxy(req)
				return resp, nil
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

func CreateActivity(activityType string, obj activitypub.ObjectBase) (activitypub.Activity, error) {
	var newActivity activitypub.Activity

	actor, err := FingerActor(obj.Actor)
	if err != nil {
		return newActivity, err
	}

	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = &actor
	newActivity.Object = &obj

	for _, e := range obj.To {
		if obj.Actor != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor != e {
			newActivity.Cc = append(newActivity.Cc, e)
		}
	}

	return newActivity, nil
}

func AddFollowersToActivity(activity activitypub.Activity) (activitypub.Activity, error) {
	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		aFollowers, err := GetActorCollection(e + "/followers")
		if err != nil {
			return activity, err
		}

		for _, k := range aFollowers.Items {
			activity.To = append(activity.To, k.Id)
		}
	}

	var nActivity activitypub.Activity

	for _, e := range activity.To {
		var alreadyTo = false
		for _, k := range nActivity.To {
			if e == k || e == activity.Actor.Id {
				alreadyTo = true
			}
		}

		if !alreadyTo {
			nActivity.To = append(nActivity.To, e)
		}
	}

	activity.To = nActivity.To

	return activity, nil
}

func IsValidActor(id string) (activitypub.Actor, bool, error) {
	actor, err := FingerActor(id)
	return actor, actor.Id != "", err
}

func AddInstanceToIndexDB(actor string) error {
	// TODO: completely disabling this until it is actually reasonable to turn it on
	// only actually allow this when it more or less works, i.e. can post, make threads, manage boards, etc
	return nil

	//sleep to be sure the webserver is fully initialized
	//before making finger request
	time.Sleep(15 * time.Second)

	nActor, err := FingerActor(actor)
	if err != nil {
		return err
	}

	if nActor.Id == "" {
		return nil
	}

	// TODO: maybe allow different indexes?
	followers, err := activitypub.GetCollectionFromID("https://fchan.xyz/followers")
	if err != nil {
		return err
	}

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == nActor.Id {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		return activitypub.AddFollower("https://fchan.xyz", nActor.Id)
	}

	return nil
}
