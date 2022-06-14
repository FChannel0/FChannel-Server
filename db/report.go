package db

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

type Reports struct {
	ID     string
	Count  int
	Actor  activitypub.Actor
	Object activitypub.ObjectBase
	OP     string
	Reason []string
}

type Report struct {
	ID     string
	Reason string
}

type Removed struct {
	ID    string
	Type  string
	Board string
}

func CloseLocalReport(id string, board string) error {
	query := `delete from reported where id=$1 and board=$2`
	_, err := config.DB.Exec(query, id, board)

	return util.MakeError(err, "CloseLocalReportDB")
}

func CreateLocalDelete(id string, _type string) error {
	var i string

	query := `select id from removed where id=$1`
	if err := config.DB.QueryRow(query, id).Scan(&i); err != nil {
		query := `insert into removed (id, type) values ($1, $2)`
		if _, err := config.DB.Exec(query, id, _type); err != nil {
			return util.MakeError(err, "CreateLocalDeleteDB")
		}
	}

	query = `update removed set type=$1 where id=$2`
	_, err := config.DB.Exec(query, _type, id)

	return util.MakeError(err, "CreateLocalDeleteDB")
}

func CreateLocalReport(id string, board string, reason string) error {
	query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`
	_, err := config.DB.Exec(query, id, 1, board, reason)

	return util.MakeError(err, "CreateLocalReportDB")
}

func GetLocalDelete() ([]Removed, error) {
	var deleted []Removed

	query := `select id, type from removed`
	rows, err := config.DB.Query(query)

	if err != nil {
		return deleted, util.MakeError(err, "GetLocalDeleteDB")
	}

	defer rows.Close()
	for rows.Next() {
		var r Removed

		if err := rows.Scan(&r.ID, &r.Type); err != nil {
			return deleted, util.MakeError(err, "GetLocalDeleteDB")
		}

		deleted = append(deleted, r)
	}

	return deleted, nil
}

func GetLocalReport(board string) (map[string]Reports, error) {
	var reported = make(map[string]Reports)

	query := `select id, reason from reported where board=$1`
	rows, err := config.DB.Query(query, board)

	if err != nil {
		return reported, util.MakeError(err, "GetLocalReportDB")
	}

	defer rows.Close()
	for rows.Next() {
		var r Report

		if err := rows.Scan(&r.ID, &r.Reason); err != nil {
			return reported, util.MakeError(err, "GetLocalReportDB")
		}

		if report, has := reported[r.ID]; has {
			report.Count += 1
			report.Reason = append(report.Reason, r.Reason)
			reported[r.ID] = report
			continue
		}

		var obj = activitypub.ObjectBase{Id: r.ID}

		col, _ := obj.GetCollectionFromPath()

		OP, _ := obj.GetOP()

		reported[r.ID] = Reports{
			ID:     r.ID,
			Count:  1,
			Object: col.OrderedItems[0],
			OP:     OP,
			Actor:  activitypub.Actor{Name: board, Outbox: config.Domain + "/" + board + "/outbox"},
			Reason: []string{r.Reason},
		}
	}

	return reported, nil
}

type ReportsSortDesc []Reports

func (a ReportsSortDesc) Len() int           { return len(a) }
func (a ReportsSortDesc) Less(i, j int) bool { return a[i].Object.Updated.After(a[j].Object.Updated) }
func (a ReportsSortDesc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
