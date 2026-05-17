package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DavidHoenisch/remotr/agent/settings"
)

var (
	AgentSettings *settings.Settings
	once          sync.Once
)

// init singleton instance of agent settings
func initSettings() {
	once.Do(func() {
		settings, err := settings.SettingsFromDefault()
		if err != nil {
			log.Fatal("Could not init settings")
		}

		AgentSettings = settings
	})
}

// main entry point for the agent
func main() {

	// init singleton settings instnace
	initSettings()

	ticker := time.NewTicker(time.Duration(AgentSettings.Agent.SyncFrequency))

	done := make(chan bool)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// start the main loop
	go func() {
		for {
			select {
			case <-done:
				// TODO: shutdown logic here
				return
			case t := <-ticker.C:
				fmt.Println(t)
			}
		}
	}()

	// watch for kill sigs
	go func() {
		for {
			sig := <-sigChan
			switch sig {
			case syscall.SIGINT | syscall.SIGHUP | syscall.SIGTERM:
				done <- true
			}
		}
	}()
}
