package webfinger

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/util"
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

func GetActorByNameFromBoardCollection(name string) activitypub.Actor {
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
			return collection, util.MakeError(err, "GetBoardCollection")
		}

		if boardActor.Id == "" {
			boardActor, err = activitypub.FingerActor(e.Id)

			if err != nil {
				return collection, util.MakeError(err, "GetBoardCollection")
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
	var actor string

	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)

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
		return nActor, util.MakeError(err, "GetActorFromPath")
	}

	if nActor.Id == "" {
		nActor = GetActorByNameFromBoardCollection(actor)
	}

	return nActor, nil
}

func StartupArchive() error {
	for _, e := range FollowingBoards {
		actor, err := activitypub.GetActorFromDB(e.Id)

		if err != nil {
			return util.MakeError(err, "StartupArchive")
		}

		if err := actor.ArchivePosts(); err != nil {
			return util.MakeError(err, "StartupArchive")
		}
	}

	return nil
}
