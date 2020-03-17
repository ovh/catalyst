# Influx

The full documentation is available at [https://docs.influxdata.com/influxdb/v1.2/guides/writing_data/](https://docs.influxdata.com/influxdb/v1.2/guides/writing_data/){.external}

## Authentification

To push data to Warp 10 with catalyst, you will need a **WRITE TOKEN**. Use Basic Auth directly inside the URL to pass it properly, like this :

<pre>http://user:[WRITE_TOKEN]@127.0.0.1:9105/influxdb</pre>

## Pushing datapoints using cURL

```shell-session
 $ curl -i -XPOST \
     'http://user:[WRITE_TOKEN]@127.0.0.1:9105/influxdb/write' \
     --data-binary \
     'cpu_load_short,host=server01,region=us-west value=0.64 1434055562000000000'
```
