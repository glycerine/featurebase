package main

import (
	"log"
	"os"

	"github.com/jaffee/commandeer/pflag"
	"github.com/molecula/featurebase/v3/idk/kinesis"
	"github.com/molecula/featurebase/v3/logger"
)

func main() {
	m := kinesis.NewMain()
	if err := pflag.LoadEnv(m, "CONSUMER_", nil); err != nil {
		log.Fatal(err)
	}
	m.Rename()
	if m.DryRun {
		log.Printf("%+v\n", m)
		return
	}
	if err := m.Run(); err != nil {
		log := m.Log()
		if log == nil {
			// if we fail before a logger was instantiated
			logger.NewStandardLogger(os.Stderr).Errorf("Error running command: %v", err)
			os.Exit(1)
		}
		log.Errorf("Error running command: %v", err)
		os.Exit(1)
	}
}
