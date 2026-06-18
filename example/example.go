package main

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/tristanfisher/patchpanel"
)

type DatabaseConfig struct {
	Host     string `default:"localhost"`
	Port     int    `default:"5432"`
	Name     string `default:"myDatabase"`
	User     string `default:"myUser"`
	Password string `default:"myPassword"`
	SSLMode  string `default:"disable"`
}

type Config struct {
	Runtime    time.Duration `default:"5s"`
	StringTest string        `default:"hi"`
	IntValue   int           `default:"42"`
	DBConfig   DatabaseConfig
}

func ParseConfig(configPath string, configStruct Config) (*Config, error) {
	patch := patchpanel.NewPatchPanel(patchpanel.TokenSeparator, patchpanel.KeyValueSeparator)

	// get defaults off of our struct using patchpanel
	confType := reflect.TypeOf(configStruct)
	for i := 0; i < confType.NumField(); i++ {
		fieldVal, err := patch.GetDefault(confType.Field(i).Name, confType, []string{})
		if err != nil {
			return &Config{}, err
		}
		fmt.Println(confType.Field(i).Name, " => ", fieldVal)
	}
	return &configStruct, nil
}

func main() {
	// grab a values/configuration file path from our environment using patchpanel
	valuesFile := patchpanel.GetFileEnvOrPath(patchpanel.ENV_CONFIG_FILE, patchpanel.FLAG_CONFIG_FILE)
	conf, err := ParseConfig(valuesFile, Config{})
	if err != nil {
		_, _ = os.Stderr.WriteString(fmt.Sprintf("error parsing configuration: %s\n", err.Error()))
		os.Exit(1)
	}
	fmt.Println(conf)
}
