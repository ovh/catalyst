# Prometheus

Prometheus is using the [pull-based approach](https://prometheus.io/docs/introduction/faq/#why-do-you-pull-rather-than-push?){.external} to gather metrics. We developed an open-source tool called [`Beamium`)(https://github.com/ovh/beamium/) in order to scrape metrics in Prometheus format and to push them to Warp10.

## How to Push data with Prometheus PushGateway

To push data to Warp10, you will need a valid **WRITE TOKEN**. Catalyst supports the [PushGateway](https://prometheus.io/docs/instrumenting/pushing/){.external} with the following URL:

<pre>http://user:[WRITE_TOKEN]@127.0.0.1:9105/prometheus</pre>

## Prometheus remote write

To activate prometheus remote write with catalyst, edit `prometheus.yml` configuration file, this file contains the global instance configuration.

You can find this file by greping the process which use it.

```sh
ps aux | grep prometheus | grep -v 'grep'
```

The process arg `--config.file` contains the configuration file path.

To setup prometheus remote configuration add the following lines (replace WRITE_TOKEN by a valid Warp10 token):

```yaml
remote_write:
  - url: http://127.0.0.1:9105/prometheus/remote_write
    basic_auth:
      username: ''
      password: 'WRITE_TOKEN'
```

Don't forget to restart your Prometheus instance to apply modifications.
