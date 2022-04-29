package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/db"
)

type PageData struct {
	Title             string
	PreferredUsername string
	Board             db.Board
	Pages             []int
	CurrentPage       int
	TotalPage         int
	Boards            []db.Board
	Posts             []activitypub.ObjectBase
	Key               string
	PostId            string
	Instance          activitypub.Actor
	InstanceIndex     []activitypub.ObjectBase
	ReturnTo          string
	NewsItems         []db.NewsItem
	BoardRemainer     []int
	Meta              Meta

	Themes      *[]string
	ThemeCookie string
}

type AdminPage struct {
	Title         string
	Board         db.Board
	Key           string
	Actor         string
	Boards        []db.Board
	Following     []string
	Followers     []string
	Reported      []db.Report
	Domain        string
	IsLocal       bool
	PostBlacklist []db.PostBlacklist
	AutoSubscribe bool

	Themes      *[]string
	ThemeCookie string
}

type Meta struct {
	Title       string
	Description string
	Url         string
	Preview     string
}
