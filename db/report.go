package db

import "github.com/FChannel0/FChannel-Server/config"

type Report struct {
	ID     string
	Count  int
	Reason string
}

type Removed struct {
	ID    string
	Type  string
	Board string
}

func CreateLocalDeleteDB(id string, _type string) error {
	query := `select id from removed where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		var i string

		if err := rows.Scan(&i); err != nil {
			return err
		}

		if i != "" {
			query := `update removed set type=$1 where id=$2`

			if _, err := config.DB.Exec(query, _type, id); err != nil {
				return err
			}
		}
	} else {
		query := `insert into removed (id, type) values ($1, $2)`

		if _, err := config.DB.Exec(query, id, _type); err != nil {
			return err
		}
	}

	return nil
}

func GetLocalDeleteDB() ([]Removed, error) {
	var deleted []Removed

	query := `select id, type from removed`

	rows, err := config.DB.Query(query)
	if err != nil {
		return deleted, err
	}

	defer rows.Close()

	for rows.Next() {
		var r Removed

		if err := rows.Scan(&r.ID, &r.Type); err != nil {
			return deleted, err
		}

		deleted = append(deleted, r)
	}

	return deleted, nil
}

func CreateLocalReportDB(id string, board string, reason string) error {
	query := `select id, count from reported where id=$1 and board=$2`

	rows, err := config.DB.Query(query, id, board)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		var i string
		var count int

		if err := rows.Scan(&i, &count); err != nil {
			return err
		}

		if i != "" {
			count = count + 1
			query := `update reported set count=$1 where id=$2`

			if _, err := config.DB.Exec(query, count, id); err != nil {
				return err
			}
		}
	} else {
		query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		if _, err := config.DB.Exec(query, id, 1, board, reason); err != nil {
			return err
		}
	}

	return nil
}

func GetLocalReportDB(board string) ([]Report, error) {
	var reported []Report

	query := `select id, count, reason from reported where board=$1`

	rows, err := config.DB.Query(query, board)
	if err != nil {
		return reported, err
	}
	defer rows.Close()

	for rows.Next() {
		var r Report

		if err := rows.Scan(&r.ID, &r.Count, &r.Reason); err != nil {
			return reported, err
		}

		reported = append(reported, r)
	}

	return reported, nil
}

func CloseLocalReportDB(id string, board string) error {
	query := `delete from reported where id=$1 and board=$2`

	_, err := config.DB.Exec(query, id, board)
	return err
}
