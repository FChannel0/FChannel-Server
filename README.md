# About

FChannel is a [libre](https://en.wikipedia.org/wiki/Free_and_open-source_software), [self-hostable](https://en.wikipedia.org/wiki/Self-hosting_(web_services)), [federated](https://en.wikipedia.org/wiki/Federation_(information_technology)), [imageboard](https://en.wikipedia.org/wiki/Imageboard) platform that utilizes [ActivityPub](https://activitypub.rocks/).

There are currently several instances federated with each other, for a full list see: https://fchan.xyz

There is an anon testing FChannel instances on TOR/Loki/I2P. Find more information here: https://fchan.xyz/g/MORL0KUT
It is a testing environment, so the instances might come and go.

## To Do List
Current things that will be implemented first are:
- A way to automatically index new instances into a list so others can discover instances as they come online.
- Setting up a server proxy so that clearnet instances can talk to TOR/Loki/I2P instances.
- Other improvements will be made over time, first it needs to be as easy as possible for new instances to come online and connect with others reliably.

Try and run your own instances and federate with one of the instances above.
Any contributions or suggestions are appreciated. Best way to give immediate feedback is the XMPP: general@rooms.fchannel.org or Matrix: #fchan:matrix.org

# Server Installation and Configuration

## Minimum Server Requirements

- Go v1.16+

- PostgreSQL

- Redis

- ImageMagick

- exiv2

## Server Installation Instructions

- Ensure you have golang installed and set a correct `GOPATH`

- `git clone` the software

- Copy `config-init` to `config` and change the values appropriately to reflect the instance.

- Create the database, username, and password for psql that is used in the `config` file.

- Start the server with `go run .`.

## Server Configuration

### config file

  `instance:fchan.xyz`  Domain name that the host can be located at without www and `http://` or `https://`

  `instancetp:https://` Transfer protocol your domain is using, should be https if possible. Do not put `https://` if you are using `http://`

  `instanceport:3000`   Port the server is running on locally, on your server.

  `instancename:FChan`  Full name that you want your instances to be called.

  `instancesummary:FChan is a federated image board instance.` Brief description of your instance.


  `dbhost:localhost`    Database host. Most likely leave as `localhost`.

  `dbport:5432`         Port number for database. Most likely leave the default value.

  `dbname:fchan_server` Database name for psql.

  `dbuser:admin`        Database user that can connect to dbname.

  `dbpass:password`     Database password for dbuser.

  `torproxy:127.0.0.1:9050`     Tor proxy route and port, leave blank if you do not want to support

  `publicindex:true`     Add instance to the public instance index at https://fchan.xyz, if you do not want to be added set this value to `false` If you are on the public index and want to be removed get in contact with the FChan dev to be removed.

  `instancesalt:put your salt string here` Used for secure tripcodes currently.


  Currently e-mail is not implemented to do anything special, but the code is in place

  `emailserver:mail.fchan.xyz`

  `emailport:465`

  `emailaddress:contact@fchan.xyz`

  `emailpass:password`

### Local testing

  When testing on a local env when setting the `instance` value in the config file you have to append the port number to the local address eg. `instance:localhost:3000` with `instanceport` also being set to the same port.

  If you want to test federation between servers locally you have to use your local ip as the `instance` eg. `instance:192.168.0.2:3000` and `instance:192.168.0.2:4000` adding the port to localhost will not route correctly.

### Managing the server

  To access the managment page to create new boards or subscribe to other boards, when you start the server the console will output the `Mod key` and `Admin Login`
  Use the `Mod key` by appending it to your server's url, `https://fchan.xyz/[Mod key]` once there you will be prompted for the `Admin Login` credentials.
  You can manage each board by appending the `Mod key` to the desired board url: `https://fchan.xyz/[Mod Key]/g`
  The `Mod key` is not static and is reset on server restart.

## Server Update

 Check the git repo for the latest commits. If there are commits you want to update to, git pull and restart the instance.

## Networking

### NGINX Template

Use [Certbot](https://github.com/certbot/certbot), (or your tool of choice) to setup SSL.

```
server {
        listen 80;
        listen [::]:80;

        root /var/www/html;

        server_name DOMAIN_NAME;

        client_max_body_size 100M;

        location / {
                # First attempt to serve request as file, then
                # as directory, then fall back to displaying a 404.
                #try_files $uri $uri/ =404;
                proxy_pass http://localhost:3000;
                proxy_http_version 1.1;
                proxy_set_header Upgrade $http_upgrade;
                proxy_set_header Connection 'upgrade';
                proxy_set_header Host $host;
                proxy_set_header X-Real-IP $remote_addr;
                proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
                proxy_set_header X-Forwarded-Proto $scheme;
                proxy_cache_bypass $http_upgrade;
        }
}
```

#### Using Certbot With NGINX

- After installing Certbot and the Nginx plugin, generate the certificate: `sudo certbot --nginx --agree-tos --redirect --rsa-key-size 4096 --hsts --staple-ocsp --email YOUR_EMAIL -d DOMAIN_NAME`

- Add a job to cron so the certificate will be renewed automatically: `echo "0 0 * * *  root  certbot renew --quiet --no-self-upgrade --post-hook 'systemctl reload nginx'" | sudo tee -a /etc/cron.d/renew_certbot`

### Apache

`Please consider submitting a pull request if you set up a FChannel instance with Apache with instructions on how to do so`

### Caddy

`Please consider submitting a pull request if you set up a FChannel instance with Caddy with instructions on how to do so`

### Docker

`Please consider submitting a pull request if you set up a FChannel instance with Docker with instructions on how to do so`

# Support

Any support is appreciated all funds go to hosting and development of the project

XMR - 85ma5KYR8Jk8zhGospQ8DeMNUrY74rQqEgiiPHvKHbowa37TAa5MLUD8RBaupw5oAxWmpFDrSAxsDbeXcfoAwiZF69mq4CE
