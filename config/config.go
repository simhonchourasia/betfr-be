package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

// Add field here when new config element in json
type Config struct {
	MongoURI        string `json:"mongoURI"`
	Cluster         string `json:"cluster"`
	UserCollection  string `json:"userCollection"`
	BetCollection   string `json:"betCollection"`
	StakeCollection string `json:"stakeCollection"`
	SecretKey       string `json:"secretKey"`
	Domain          string `json:"domain"`
	Port            string `json:"port"`
	Debug           bool   `json:"debug"`
}

var configOnce sync.Once
var GlobalConfig Config
var cfgErr error = nil

func SetupConfig() error {
	configOnce.Do(func() {
		log.Println("Reading config file...")
		jsonFile, err := os.Open("config/default.json")
		defer jsonFile.Close()
		if err != nil {
			cfgErr = err
		} else {
			configBytes, err := ioutil.ReadAll(jsonFile)
			if err != nil {
				cfgErr = err
			} else {
				json.Unmarshal(configBytes, &GlobalConfig)
			}
		}
	})
	return cfgErr
}
