# OpenTSDB

## Authentification

To push data to Warp10, you will need a valid **WRITE TOKEN**. Use Basic Auth directly inside the URL to pass it properly, like this:

<pre>http://user:[WRITE_TOKEN]@127.0.0.1:9105/opentsdb/</pre>

## Push datapoints using curl

The full documentation is available at [http://opentsdb.net/docs/build/html/api_http/put.html](http://opentsdb.net/docs/build/html/api_http/put.html){.external}. As an example you can push single point.

Create a file on your disk named `opentsdb.json`, and populate it with the following content:

```json
{
	"metric": "sys.cpu.nice",
	"timestamp": 1346846400,
	"value": 18,
	"tags": {
		"host": "web01",
		"dc": "lga"
	}
}
```

To query data on Warp 10, only the valid token set as the password of the basic authentication will be used.

```shell-session
$ curl -X POST -d @opentsdb.json 'http://user:[WRITE_TOKEN]@127.0.0.1:9105/opentsdb/api/put'
```

If everyting happens correctly, the CURL would exit with a 200 code status.

## Push datapoints using Python

Here's an example how you can use Python to push datapoints in OpenTSDB format using [Requests](http://docs.python-requests.org/en/master/){.external}:

```python
>>> import requests

>>> url = 'http://user:[WRITE_TOKEN]@127.0.0.1:9105/opentsdb/api/put'
>>> payload = {}
>>> payload["metric"] = "sys.cpu.nice"
>>> payload["timestamp"] = "1346846400"
>>> payload["value"] = 18
>>> tags = { "host": "web01", "dc": "lga"}
>>> payload["tags"] = tags

>>> r = requests.post(url, json=payload)
>>> r.status_code
```
