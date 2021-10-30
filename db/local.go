package db

func IsIDLocal(id string) (bool, error) {
	activity, err := GetActivityFromDB(id)
	return len(activity.OrderedItems) > 0, err
}

func IsActorLocal(id string) (bool, error) {
	actor, err := GetActorFromDB(id)
	return actor.Id != "", err
}

func IsObjectLocal(id string) (bool, error) {
	query := `select id from activitystream where id=$1`

	rows, err := db.Query(query, id)
	if err != nil {
		return false, err
	}

	var nID string
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&nID)
	return nID != "", err
}

func IsObjectCached(id string) (bool, error) {
	query := `select id from cacheactivitystream where id=$1`
	rows, err := db.Query(query, id)
	if err != nil {
		return false, err
	}

	var nID string
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&nID)
	return nID != "", err
}
