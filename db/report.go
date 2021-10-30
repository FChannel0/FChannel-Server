package db

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
