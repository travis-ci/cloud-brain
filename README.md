# Cloud Brain

**HERE BE DRAGONS**

(Seriously, all this is very WIP, and things might move around a lot.)

Cloud Brain is a service that provides a uniform API endpoint for all cloud compute services that Travis CI interacts with.

This allows for instance creation, metadata, and deletion to be managed in a central place. It allows for generic cleanup tasks to be created. It allows for [worker](https://github.com/travis-ci/worker) to talk to a single service. And finally, it allows for pluggable cloud backends to be implemented, in order to support additional infrastructures.

## Architecture

There are two main parts of Cloud Brain: An HTTP API, and a background worker. The HTTP API does not interact with the compute providers directly, it will queue jobs for the background worker to do that.

The codebase is divided into a few different subpackages:

- `cbcontext`: Contains some wrappers around the [context](http://godoc.org/golang.org/x/net/context) package, which is used all over the remainder of the codebase.
- `cloud`: Contains the implementations for the various cloud providers.
- `cloudbrain`: Contains the "main business logic". Should, generally speaking, be the main entry point for any API calls. The `http` package should only do HTTP-related things and then call this.
- `cmd`: Contains a subpackage for each binary to generate.
  - `cloudbrain-create-token`: Creates an authentication token and pushes it to the database.
  - `cloudbrain-create-worker`: Runs the worker that processes create events, and creates the instances on the cloud provider(s).
  - `cloudbrain-http`: Runs the HTTP API.
  - `cloudbrain-refresh-worker`: Runs the worker that synchronizes the state of the database with the state at the provider(s).
- `database`: Contains all the database-specific logic.
- `http`: Contains the HTTP API logic. This should only do HTTP-specific things (like serialization and specific HTTP errors), but should call into the `cloudbrain` package for the actual business logic.
- `sqitch`: Not a Go package, but contains all the files for [Sqitch](http://sqitch.org/), which is used for database migrations.

## HTTP API

### Authentication

The authentication is token-based, and backed by the database. The tokens themselves aren't stores in the database, only a hashed version using scrypt is stored there.

To generate a token, use the `cloudbrain-create-token` tool:

```
$ cloudbrain-create-token "description of the token"
generated token: 1-b180349faf82840b43ebf27e730f894f
```

This will generate the token locally, connect to the database (remember to set the `DATABASE_URL` environment variable), and upload the salt and hash (which are also computed locally).

The tokens are on the form `id-token`, where the `id` is a numerical ID that the server uses to look up the salt and hash in the database.

To authenticate with the API, pass the token in the `Authorization` header like this:

``` HTTP
Authorization: token 1-b180349faf82840b43ebf27e730f894f
```

If the token is in any way invalid, a 401 will be returned.

### Create instance

```
POST /instances
```

#### Input

| Name             | Type     | Description |
| ---------------- | -------- | ----------- |
| `provider`       | `string` | **Required**. The name of the provider to create the instance on. `gce` is the only currently supported provider. |
| `image`          | `string` | **Required**. The name of the image to use to create the instance. |
| `instance_type`  | `string` | Either `standard` (the default) or `premium`, depending on what kind of VM you'd like to start. May not be supported by all providers. |
| `public_ssh_key` | `string` | The public SSH key to inject into the VM for SSH access. May not be supported by all providers. |

#### Example

``` JSON
{
	"provider": "gce",
	"image": "image-2016-01-01",
	"instance_type": "standard",
	"public_ssh_key": "ssh-rsa …"
}
```

#### Response

```
Status: 201 Created
Location: https://cloud-brain/instances/0d654ef4-75b9-49a6-9f90-f9b1ae3501fc
```

``` JSON
{
	"id": "0d654ef4-75b9-49a6-9f90-f9b1ae3501fc",
	"provider": "gce",
	"image": "image-2016-01-01",
	"instance_type": "standard",
	"public_ssh_key": "ssh-rsa …",
	"ip_address": null,
	"state": "creating"
}
```

### Get instance information

```
GET /instances/:uuid
```

#### Response

The `state` can be one of: `creating`, `starting`, `running`, `terminating`.

```
Status: 200 OK
```

``` JSON
{
	"id": "0d654ef4-75b9-49a6-9f90-f9b1ae3501fc",
	"provider": "gce",
	"image": "image-2016-01-01",
	"ip_address": "203.0.113.175",
	"state": "running"
}
```

## Usage (script)

There is a nice client script that you can use to interact with the API. It uses [httpie](https://github.com/jkbrzt/httpie).

Since httpie supports `~/.netrc` files for authentication, you can create such a file (if it does not already exist), and configure it with your cloud-brain token as follows:

```
machine travis-cloud-brain-staging.herokuapp.com
  login token
  password 0-your-very-secret-cloud-brain-token
```

Example:

```bash
$ script/cloud-brain-staging post instances provider=gce-staging image=travis-ci-amethyst-trusty-1470801111 instance_type=standard
$ script/cloud-brain-staging get instances/6f6466f9-b99b-4b7f-b446-6d85ce4c8958
$ script/cloud-brain-staging delete instances/6f6466f9-b99b-4b7f-b446-6d85ce4c8958
```
