# About

FChannel is a [libre](https://en.wikipedia.org/wiki/Free_and_open-source_software), [self-hostable](https://en.wikipedia.org/wiki/Self-hosting_(web_services)), [federated](https://en.wikipedia.org/wiki/Federation_(information_technology)), [imageboard](https://en.wikipedia.org/wiki/Imageboard) platform that utilizes [ActivityPub](https://activitypub.rocks/). 

# Server Installation and Configuration

## Minimum Server Requirements

_POST MINIMUM SERVER REQUIREMENTS HERE (OS, Hardware, etc.) HERE_ 

## Server Installation Instructions

- Ensure you have golang installed at a correct `GOPATH`

- Copy `config-init` to `config` and change the values appropriately to reflect the instance.

- Create the database, username, and password for psql that is used in config file.

- Run `psql -U (user) -d (database) -f databaseschema.psql`

- Finally start the server with `go run`.

## Server Configuration

_POST VARIOUS NOTABLE CONFIG OPTIONS THE HOST MAY WANT TO CHANGE IN CONFIGURATION, AS WELL AS ANY SECURITY SETTINGS HERE._ 

## Server Update

_PROVIDE INSTRUCTIONS FOR UPDATING THE SERVER TO THE LATEST VERSION HERE._

## Networking

### NGINX Template

_PROVIDE A BASIC NGINX TEMPLATE TWEAKED FOR FCHANNEL HERE_

### Apache

_PROVIDE A BASIC APACHE TEMPLATE TWEAKED FOR FCHANNEL HERE_

### Caddy

_PROVIDE A BASIC CADDYv2 TEMPLATE TWEAKED FOR FCHANNEL HERE_
