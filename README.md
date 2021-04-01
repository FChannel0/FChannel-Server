# About

FChannel is a libre, self-hostable, federated, imageboard platform that utilizes ActivittPub. 

# Server Installation and Configuration

## Minimum Server Requirements

_POST MINIMUM SERVER REQUIREMENTS HERE (OS, Hardware, etc.) HERE_ 

## Server Installation Instructions

- Ensure you have golang installed at a correct 
```
GOPATH
```

- Copy `config-init` to `config` and change the values appropriately to reflect the instance.

- Create the database, username, and password for psql that is used in config file.

- Run `psql -U (user) -d (database) -f databaseschema.psql`

- Finally start the server with `go run`.

## Server Configuration

_POST VARIOUS NOTABLE CONFIG OPTIONS THE HOST MAY WANT TO CHANGE IN CONFIGURATION, AS WELL AS ANY SECURITY SETTINGS HERE._ 

## Networking

### NGINX Template

_PROVIDE A BASIC NGINX TEMPLATE HERE_

### Apache

_PROVIDE A BASIC APACHE TEMPLATE HERE_

### Caddy

_PROVIDE A BASIC CADDYv2 TEMPLATE HERE_