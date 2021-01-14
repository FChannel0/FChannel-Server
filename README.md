Setup:

Install golang

Set these enviroment variables:
export GOROOT=/usr/lib/go
export GOPATH=$HOME/.local/go //or where ever you have you go src dir
export PATH="$PATH:$GOPATH/bin"

run
go get github.com/lib/pq //database

create a database and user with psql and run
psql -U (user) -d (database) -f databaseschema.psql

set db user, password, name in main.go
set the Domain variable to the domain name that identifies this instance

run
go run .

One up and running query the database with
select * from boardaccess;
To be able to get your administrative credentials
