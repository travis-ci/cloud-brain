# Cloud Brain

**HERE BE DRAGONS**

(Seriously, all this is very WIP, and things might move around a lot.)

Cloud Brain is a service that provides a uniform API endpoint for all cloud compute services that Travis CI interacts with.

## Architecture

There are two main parts of Cloud Brain: An HTTP API, and a background worker. The HTTP API does not interact with the compute providers directly, it will queue jobs for the background worker to do that.

## HTTP API

### Create instance

```
POST /instances
```

``` JSON
{
	"provider": "gce",
	"image": "image-2016-01-01"
}
```

#### Input

| Name       | Type     | Description |
| ---------- | -------- | ----------- |
| `provider` | `string` | **Required**. The name of the provider to create the instance on. `gce` is the only currently supported provider. |
| `image`    | `string` | **Required**. The name of the image to use to create the instance. |

#### Example

``` JSON
{
	"provider": "gce",
	"image": "image-2016-01-01"
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
