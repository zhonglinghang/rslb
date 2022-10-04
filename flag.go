package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
)

type Mobility struct {
	Enable   bool `json:"enable"`
	Mode     int  `json:"mode"`
	Interval int  `json:"interval_s"`
}

type Config struct {
	ListenAddr string   `json:"listen"`
	SFUList    []string `json:"sfu_list"`
	Mobility   Mobility `json:"mobility"`
	Debug      string   `json:"debug"`
}

func (v *Config) Parse(conf string) error {
	f, err := os.Open(conf)
	if err != nil {
		return errors.New("fails to open config file")
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return errors.New("fails to read config file")
	}

	if err := json.Unmarshal(b, v); err != nil {
		return errors.New("unmarshal config json")
	}
	return nil
}

func parseArgs(ctx context.Context, args []string) (config *Config, err error) {
	cl := flag.NewFlagSet(args[0], flag.ContinueOnError)

	var conf string
	cl.StringVar(&conf, "c", "", "The config file")
	cl.StringVar(&conf, "conf", "", "The file to load config from")

	if err = cl.Parse(args[1:]); err != nil {
		return
	}

	// load config file
	if conf != "" {
		c := &Config{}
		if err = c.Parse(conf); err != nil {
			log.Error("parse config file %s", conf)
			err = errors.New("parse config file")
			return
		}
		config = c
		return
	}

	return nil, errors.New("no config file")
}
