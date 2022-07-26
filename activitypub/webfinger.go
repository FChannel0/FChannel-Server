package activitypub

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

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

func GetActor(id string) (Actor, error) {
	var respActor Actor

	if id == "" {
		return respActor, nil
	}

	actor, instance := GetActorAndInstance(id)

	if ActorCache[actor+"@"+instance].Id != "" {
		respActor = ActorCache[actor+"@"+instance]
		return respActor, nil
	}

	req, err := http.NewRequest("GET", strings.TrimSpace(id), nil)
	if err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)

	if err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &respActor); err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	ActorCache[actor+"@"+instance] = respActor

	return respActor, nil
}

//looks for actor with pattern of board@instance
func FingerActor(path string) (Actor, error) {
	var nActor Actor

	actor, instance := GetActorAndInstance(path)

	if actor == "" && instance == "" {
		return nActor, nil
	}

	if ActorCache[actor+"@"+instance].Id != "" {
		nActor = ActorCache[actor+"@"+instance]
	} else {
		resp, err := FingerRequest(actor, instance)
		if err != nil {
			return nActor, util.MakeError(err, "FingerActor finger request")
		}

		if resp != nil && resp.StatusCode == 200 {
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nActor, util.MakeError(err, "FingerActor read resp")
			}

			if err := json.Unmarshal(body, &nActor); err != nil {
				return nActor, util.MakeError(err, "FingerActor unmarshal")
			}

			ActorCache[actor+"@"+instance] = nActor
		}
	}

	return nActor, nil
}

func FingerRequest(actor string, instance string) (*http.Response, error) {
	acct := "acct:" + actor + "@" + instance

	// TODO: respect https
	req, err := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)

	if err != nil {
		return nil, util.MakeError(err, "FingerRequest")
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return resp, err
	}

	var finger Webfinger

	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return resp, err
		}

		if err := json.Unmarshal(body, &finger); err != nil {
			return resp, util.MakeError(err, "FingerRequest")
		}
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, err = http.NewRequest("GET", e.Href, nil)

				if err != nil {
					return resp, util.MakeError(err, "FingerRequest")
				}

				break
			}
		}
	}

	req.Header.Set("Accept", config.ActivityStreams)
	if resp, err = util.RouteProxy(req); err != nil {
		return resp, util.MakeError(err, "FingerRequest")
	}

	return resp, nil
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
		return util.MakeError(err, "IsValidActor")
	}

	if nActor.Id == "" {
		return nil
	}

	// TODO: maybe allow different indexes?
	reqActivity := Activity{Id: "https://fchan.xyz/followers"}
	followers, err := reqActivity.GetCollection()
	if err != nil {
		return util.MakeError(err, "IsValidActor")
	}

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == nActor.Id {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		actor := Actor{Id: "https://fchan.xyz"}
		return actor.AddFollower(nActor.Id)
	}

	return nil
}
