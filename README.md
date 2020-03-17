# Catalyst

Warp 10 multi-protocole ingress proxy.

Catalyst is an input proxy for [*Warp 10*](https://www.warp10.io).
Its role is to receive metrics in the native protocoles of many Time Series databases (*InfluxDB*, *OpenTSDB*, *Prometheus*...), translate them to (*Warp 10*'s *GTS input format*)[http://www.warp10.io/apis/gts-input-format/] and send them to a *Warp 10* instance.

## Pre-install

To build and install Catalyst you need:

- A working [*go*](https://golang.org) install, with the `GOROOT`and `GOPATH` variables correctly set
- [*glide*](https://github.com/Masterminds/glide) to grab the dependencies
- `make` in order to use the `Makefile`

## Install

1. Install the tooling

    `make init`
2. Grab the dependencies

    `make dep`
3. Build Catalyst

    `make dev`

## Configure

Catalyst needs a [YAML](http://yaml.org/) conf file with only one `warp_endpoint` parameter, the target [Warp 10](https://www.warp10.io) instance.

```YAML
warp_endpoint: https://[WARP10_INSTANCE_URL]/api/v0/update
```

By default, Catalyst will look for a `config.yaml` file on:

- `/etc/catalyst/`
- `$HOME/.catalyst`
- the current path

Without a config file, Catalyst will use `http://127.0.0.1:8080/api/v0/update` as Warp 10 endpoint.

## Run Catalyst

If the confilg file is at the default location, you can simply run the Catalyst binary, `./build/catalyst`.
By default, Catalyst listens on `127.0.0.1:9100`.

```sh
$ ./build/catalyst
INFO[0000] Catalyst starting
INFO[0000] Catalyst started
INFO[0000] Listen 127.0.0.1:9100
```

If you need more complex options, use `./build/catalyst --help`:

```sh
Usage:
  catalyst [flags]
  catalyst [command]

Available Commands:
  help        Help about any command
  version     Print the version number

Flags:
      --config string   config file to use
  -h, --help            help for catalyst
  -l, --listen string   listen address (default "127.0.0.1:9100")
  -v, --log-level int   Log level (from 1 to 5) (default 4)

Use "catalyst [command] --help" for more information about a command.
```

## Status

Catalyst is currently under development.

## Contributing

Instructions on how to contribute to Catalyst are available on the [Contributing](./CONTRIBUTING.md) page.

## Metrics

Catalyst exposes metrics about his usage:

| name                                        | labels                  | type    | description                                                               |
| ------------------------------------------- | ----------------------- | ------- | ------------------------------------------------------------------------- |
| catalyst_graphite_tcp_requests_total        |                         | counter | Number of Graphite TCP requests handled.                                  |
| catalyst_graphite_tcp_requests_success      |                         | counter | Number of Graphite TCP requests in success.                               |
| catalyst_graphite_tcp_requests_errors       |                         | counter | Number of Graphite TCP requests in errors.                                |
| catalyst_graphite_tcp_requests_noauth       |                         | counter | Number of Graphite TCP requests where authentication is missing.          |
| catalyst_graphite_tcp_requests_datapoints   |                         | counter | Number of Graphite TCP pushed datapoints.                                 |
| catalyst_graphite_tcp_requests_elapsed_time |                         | counter | Graphite TCP requests elapsed time.                                       |
| catalyst_error_connreset                    |                         | counter | Number of connections reset.                                              |
| catalyst_protocol_request                   | protocol                | counter | Number of request handled on specific protocol.                           |
| catalyst_protocol_status_code               | protocol, status        | counter | Number of request handled with specific protocol and warning status code. |
| catalyst_protocol_datapoints                | protocol                | counter | Number of processed datapoints on specific protocol.                      |
| catalyst_error_mads                         | app                     | counter | Mads error count.                                                         |
| catalyst_error_ddp                          | app                     | counter | Ddp error count.                                                          |
| catalyst_error_broken_pipe                  |                         | counter | Warp broken pipes errors count.                                           |
| catalyst_bannish_request                    | token                   | counter | Number of request with this bannished token.                              |
| catalyst_bannish_current                    |                         | gauge   | Number of bannished token in current session.                             |
| catalyst_http_request                       |                         | counter | Number of http request handled.                                           |
| catalyst_http_error_request                 |                         | counter | Number of http request in error.                                          |
| catalyst_http_status_code                   | status                  | counter | Number request in status code.                                            |
| catalyst_http_response_time                 |                         | counter | Requests response time count in nanoseconds.                              |

## Licence

Catalyst is released under a [3-BSD clause license](./LICENSE).

## Get in touch

- Gitter: [metrics](https://gitter.im/ovh/metrics).
