package webfinger

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
)

var Boards []Board
var FollowingBoards []activitypub.ObjectBase

type Board struct {
	Name        string
	Actor       activitypub.Actor
	Summary     string
	PrefName    string
	InReplyTo   string
	Location    string
	To          string
	RedirectTo  string
	Captcha     string
	CaptchaCode string
	ModCred     string
	Domain      string
	TP          string
	Restricted  bool
	Post        activitypub.ObjectBase
}

type BoardSortAsc []Board

func (a BoardSortAsc) Len() int           { return len(a) }
func (a BoardSortAsc) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a BoardSortAsc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func GetActorByName(name string) activitypub.Actor {
	var actor activitypub.Actor
	boards, _ := GetBoardCollection()
	for _, e := range boards {
		if e.Actor.Name == name {
			actor = e.Actor
		}
	}

	return actor
}

func GetBoardCollection() ([]Board, error) {
	var collection []Board
	for _, e := range FollowingBoards {
		var board Board
		boardActor, err := activitypub.GetActorFromDB(e.Id)
		if err != nil {
			return collection, err
		}

		if boardActor.Id == "" {
			boardActor, err = FingerActor(e.Id)
			if err != nil {
				return collection, err
			}
		}

		board.Name = boardActor.Name
		board.PrefName = boardActor.PreferredUsername
		board.Location = "/" + boardActor.Name
		board.Actor = boardActor
		board.Restricted = boardActor.Restricted
		collection = append(collection, board)
	}

	sort.Sort(BoardSortAsc(collection))

	return collection, nil
}

func GetActorFromPath(location string, prefix string) (activitypub.Actor, error) {
	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)

	var actor string

	if len(match) < 1 {
		actor = "/"
	} else {
		actor = strings.Replace(match[1], "/", "", -1)
	}

	if actor == "/" || actor == "outbox" || actor == "inbox" || actor == "following" || actor == "followers" {
		actor = "main"
	}

	var nActor activitypub.Actor

	nActor, err := activitypub.GetActorByNameFromDB(actor)
	if err != nil {
		return nActor, err
	}

	if nActor.Id == "" {
		nActor = GetActorByName(actor)
	}

	return nActor, nil
}
