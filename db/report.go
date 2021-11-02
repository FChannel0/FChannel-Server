package db

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

func DeleteReportActivity(id string) error {
	query := `delete from reported where id=$1`

	_, err := db.Exec(query, id)
	return err
}

func ReportActivity(id string, reason string) (bool, error) {
	if res, err := IsIDLocal(id); err == nil && !res {
		// TODO: not local error
		return false, nil
	} else if err != nil {
		return false, err
	}

	actor, err := GetActivityFromDB(id)
	if err != nil {
		return false, err
	}

	query := `select count from reported where id=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return false, err
		}
	}

	if count < 1 {
		query = `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		_, err := db.Exec(query, id, 1, actor.Actor.Id, reason)
		if err != nil {
			return false, err
		}
	} else {
		count = count + 1
		query = `update reported set count=$1 where id=$2`

		_, err := db.Exec(query, count, id)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func CreateLocalDeleteDB(id string, _type string) error {
	query := `select id from removed where id=$1`

	rows, err := db.Query(query, id)
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

			if _, err := db.Exec(query, _type, id); err != nil {
				return err
			}
		}
	} else {
		query := `insert into removed (id, type) values ($1, $2)`

		if _, err := db.Exec(query, id, _type); err != nil {
			return err
		}
	}

	return nil
}

func GetLocalDeleteDB() ([]Removed, error) {
	var deleted []Removed

	query := `select id, type from removed`

	rows, err := db.Query(query)
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

	rows, err := db.Query(query, id, board)
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

			if _, err := db.Exec(query, count, id); err != nil {
				return err
			}
		}
	} else {
		query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		if _, err := db.Exec(query, id, 1, board, reason); err != nil {
			return err
		}
	}

	return nil
}

func GetLocalReportDB(board string) ([]Report, error) {
	var reported []Report

	query := `select id, count, reason from reported where board=$1`

	rows, err := db.Query(query, board)
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

	_, err := db.Exec(query, id, board)
	return err
}
