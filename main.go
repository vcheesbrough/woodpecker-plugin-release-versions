package main

import (
	"fmt"
	"log"
	"os"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/config"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/plugin"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func main() {
	var (
		cfg types.Config
		err error
	)

	if len(os.Args) > 1 {
		mode, parseErr := config.ParseMode(os.Args[1])
		if parseErr != nil {
			log.Fatal(parseErr)
		}
		cfg, err = config.FromCLI(mode)
	} else {
		cfg, err = config.FromEnv()
	}
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	if err := plugin.Run(cfg); err != nil {
		log.Fatal(fmt.Errorf("plugin failed: %w", err))
	}
}
