# Graphite

Graphite Carbon](https://github.com/graphite-project/carbon){.external} use a push-based approach using a TCP listener to gather metrics. We developed open-source tools called
`Beamium` and `Fossil` in order to push metrics to Warp 10. Please see the [dedicated guide to use Beamium](../source-beamium) and [Fossil github repository](https://github.com/ovh/fossil){.external}.

The Graphite protocol is supported on catalyst via a TCP connection or via an HTTP request. You can use the same syntax as defined in [Hosted Graphite](https://www.hostedgraphite.com/docs/#tcp-connection){.external}.

## Via a TCP connection

You can use the same syntax as defined in [Hosted Graphite](https://www.hostedgraphite.com/docs/#tcp-connection){.external}. You need to ensure each metrics names are prefixed by a valid Metric token and an "@.", like this:

```shell-session
TOKEN@.metricname value [timestamp]
```

Where TOKEN is the write token of your Warp 10 application. You can put multiple metrics on separate lines. The **timestamp** is optional.

**Host**: 127.0.0.1:9105/graphite, **Port (no SSL)**: 2003, **Port (with SSL)**: 20030

The following example shows how to send a single metric to catalyst using netcat on linux:

```shell-session
echo "TOKEN@.tcp_metric 14.2 1546420308000" | ncat 127.0.0.1:9105/graphite 2003
```

## Via StatsD in TCP

[StatsD](https://github.com/etsy/statsd){.external} is a network daemon. It listens for statistics such as counters and timers sent via UDP or TCP. You can use StatsD to perform metrics aggregations before sending them to the Warp 10.

You need to install StatsD, once it's done. StatsD can be used with a config file as describe below.

```yaml
{
  graphitePort: 2003
, graphiteHost: "127.0.0.1:9105/graphite"
, port: 8125
, backends: [ "./backends/graphite" ]
,  graphite: {
    legacyNamespace: false,
    globalPrefix: "TOKEN@"
  }
}
```

TOKEN is the write token of your Warp 10 application.

## Via an HTTP Post

You can use the same syntax as defined in [Hosted Graphite](https://www.hostedgraphite.com/docs/#http-post){.external}.
You need to ensure each metrics names have a valid Graphite format:

```shell-session
metricname value [timestamp]
```

You can put multiple metrics on separate lines. The **timestamp** is optional. You can use the following URL to push your metrics on our Graphite endpoint: http://u:TOKEN@127.0.0.1:9105/graphite/api/v1/sink.

The following example shows how to send a single metric on the gra1 REGION using the cURL command on linux:

```shell-session
curl http://u:TOKEN@127.0.0.1:9105/graphite/api/v1/sink --data-binary "https_metric 14.2 1546420308000"
```

Where TOKEN is the write token of your Warp 10 application.
