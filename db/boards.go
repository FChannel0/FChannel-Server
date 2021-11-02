package db

import (
	"sort"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/webfinger"
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

func GetBoardCollection() ([]Board, error) {
	var collection []Board
	for _, e := range FollowingBoards {
		var board Board
		boardActor, err := GetActorFromDB(e.Id)
		if err != nil {
			return collection, err
		}

		if boardActor.Id == "" {
			boardActor, err = webfinger.FingerActor(e.Id)
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
