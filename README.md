## Hostbuilder

Hostbuilder is a service that provisions box hosts.

### Endpoints

#### Create host

`POST /hosts`

**Example request**:

```
POST /hosts HTTP/1.1
Content-Type: application/x-www-form-urlencoded

provider=digitalocean&config=[JSON encoded config]
```

**Example response**:

```javascript
HTTP/1.1 201 Created
Content-Type: application/json; charset=utf-8

{
  "provider": "digitalocean",
  "id": "id-of-host",
  "ip_address": "1.2.3.4",
  "freighterd_port": 65333,
  "status": "created"
}
```

**POST Params**:

- **provider** - provider - `digitalocean`, `docker` and `amazon` are supported.
- **config** - provider-specific configuration for the host to create, json encoded. See below.

`digitalocean` provider sample config:

```javascript
{
  "name": "user-box-1",
  "image_id": "3240036", // Ubuntu 14.04 x64
  "region_id": "6", // Singapore 1
  "size_id": "66", // 512MB
  "apt_username": "dqminh", // username in nitrous.io webapp
  "apt_password": "ahahahah", // valid access token of the user
  "apt_dist": "trusty", // current distribution of the image
  "apt_location": "apt.nitrous.io", // current repository server
  "public_keys": [
    "ssh-rsa AAAAB3Nz me@macbook",
    "ssh-rsa AAAABBaZ me@example.com"
  ]
}
```

`amazon` provider sample config:

```javascript
{
  "source_ami": "ami-167b1426",
  "region": "us-west-2",
  "security_group": {
    "id": "sg-9b4c04ab",
    "name": "box_host"
  },
  "availability_zone": "us-west-2a",
  "instance_type": "t1.micro",
  "public_keys": [
    "ssh-rsa AAAAB3Nz me@macbook",
    "ssh-rsa AAAABBaZ me@example.com"
  ]
}
```

`docker` provider sample config:

```javascript
{
  "name": "user-box-1",
  "image": "nitrousio/ubuntu-dind", // docker image
  "public_keys": [
    "ssh-rsa AAAAB3Nz me@macbook",
    "ssh-rsa AAAABBaZ me@example.com"
  ]
}
```

**Status Codes**:

- **201** - created
- **400** - missing or invalid params


#### Delete host

`POST /hosts/delete`

**Example request**:

```
POST /hosts/delete HTTP/1.1
Content-Type: application/x-www-form-urlencoded

provider=digitalocean&config=[JSON encoded config]
```

**Example response**:

```javascript
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{ "status": "ok" }
```

**POST Params**:

- **provider** - provider - `digitalocean`, `docker` and `amazon` are supported.
- **config** - provider-specific configuration for the host to delete, json encoded. See below.

`digitalocean` provider sample config:

```javascript
{
  "id": "id-of-host" // ID of host on DigitalOcean
}
```

**Status Codes**:

- **200** - OK (host successfully deleted)
- **400** - missing or invalid params


#### Start host

`POST /hosts/start`

**Example request**:

```
POST /hosts/start HTTP/1.1
Content-Type: application/x-www-form-urlencoded

provider=digitalocean&config=[JSON encoded config]
```

**Example response**:

```javascript
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{ "status": "ok" }
```

**POST Params**:

- **provider** - provider - `digitalocean`, `docker` and `amazon` are supported.
- **config** - provider-specific configuration for the host to start, json encoded. See below.

`digitalocean` provider sample config:

```javascript
{
  "id": "id-of-host" // ID of host on DigitalOcean
}
```

**Status Codes**:

- **200** - OK (host successfully started)
- **400** - missing or invalid params


#### Stop host

`POST /hosts/stop`

**Example request**:

```
POST /hosts/stop HTTP/1.1
Content-Type: application/x-www-form-urlencoded

provider=digitalocean&config=[JSON encoded config]
```

**Example response**:

```javascript
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{ "status": "ok" }
```

**POST Params**:

- **provider** - provider - `digitalocean`, `docker` and `amazon` are supported.
- **config** - provider-specific configuration for the host to stop, json encoded. See below.

`digitalocean` provider sample config:

```javascript
{
  "id": "id-of-host" // ID of host on DigitalOcean
}
```

**Status Codes**:

- **200** - OK (host successfully stopped)
- **400** - missing or invalid params


#### Restart host

`POST /hosts/restart`

**Example request**:

```
POST /hosts/restart HTTP/1.1
Content-Type: application/x-www-form-urlencoded

provider=digitalocean&config=[JSON encoded config]
```

**Example response**:

```javascript
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{ "status": "ok" }
```

**POST Params**:

- **provider** - provider - `digitalocean`, `docker` and `amazon` are supported.
- **config** - provider-specific configuration for the host to restart, json encoded. See below.

`digitalocean` provider sample config:

```javascript
{
  "id": "id-of-host" // ID of host on DigitalOcean
}
```

**Status Codes**:

- **200** - OK (host successfully restarted)
- **400** - missing or invalid params

#### Availability

`GET /availability`

**Example request**:

```
GET /availability HTTP/1.1
```

**Example response**:

```javascript
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
  "digitalocean": {
    "sgp1": ["512mb", "1024mb"],
    "asm2": ["1024mb", "2048mb"]
  }
}
```
