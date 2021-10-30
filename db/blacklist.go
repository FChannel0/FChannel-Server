package db

import "regexp"

type PostBlacklist struct {
	Id    int
	Regex string
}

func WriteRegexBlacklistDB(regex string) error {
	query := `select from postblacklist where regex=$1`

	rows, err := db.Query(query, regex)
	if err != nil {
		return err
	}
	defer rows.Close()

	var re string
	rows.Next()
	rows.Scan(&re)

	if re != "" {
		return nil
	}

	query = `insert into postblacklist (regex) values ($1)`

	_, err = db.Exec(query, regex)
	return err
}

func GetRegexBlacklistDB() ([]PostBlacklist, error) {
	var list []PostBlacklist

	query := `select id, regex from postblacklist`

	rows, err := db.Query(query)
	if err != nil {
		return list, err
	}

	defer rows.Close()
	for rows.Next() {
		var temp PostBlacklist
		rows.Scan(&temp.Id, &temp.Regex)

		list = append(list, temp)
	}

	return list, nil
}

func DeleteRegexBlacklistDB(id int) error {
	query := `delete from postblacklist where id=$1`

	_, err := db.Exec(query, id)
	return err
}

func IsPostBlacklist(comment string) (bool, error) {
	postblacklist, err := GetRegexBlacklistDB()
	if err != nil {
		return false, err
	}

	for _, e := range postblacklist {
		re := regexp.MustCompile(e.Regex)

		if re.MatchString(comment) {
			return true, nil
		}
	}

	return false, nil
}
