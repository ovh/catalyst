// Catalyst Catalyst multipass proxy.
//
// Usage
//
// 		catalyst  [flags]
// Flags:
//       --config string   config file to use
//       --help            display help
//   -v, --verbose         verbose output
//   -l, --listen          listen addresse
//   -v, --log-level int   Log level (from 1 to 5)
package main

import (
	log "github.com/sirupsen/logrus"

	"github.com/ovh/catalyst/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Panicf("%v", err)
	}
}
