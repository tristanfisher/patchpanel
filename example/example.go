package main

import (
	"errors"
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
	Runtime     time.Duration `default:"5s"`
	StringTest  string        `default:"hi"`
	IntValue    int           `default:"42"`
	DBConfig    DatabaseConfig
	LuckyNumber int
	IsItSunny   bool
}

// populateDefaults recursively loads `default` struct tags into the struct pointed to by structPtr.
// When a field's type is itself a struct with no registered type parser, the struct KindParser
// returns NoValueError; populateDefaults treats this as a signal to recurse into that field.
func populateDefaults(patch *patchpanel.PatchPanel, structPtr reflect.Value) error {
	v := structPtr.Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		val, err := patch.GetDefault(field.Name, t, []string{})
		if err != nil {
			var noVal patchpanel.NoValueError
			if errors.As(err, &noVal) {
				if field.Type.Kind() == reflect.Struct {
					if err := populateDefaults(patch, fieldVal.Addr()); err != nil {
						return err
					}
				}
				continue
			}
			return err
		}

		fieldVal.Set(reflect.ValueOf(val))
		fmt.Println(field.Name, " => ", val)
	}
	return nil
}

func ParseConfig(configPath string, configStruct Config) (*Config, error) {
	patch := patchpanel.NewPatchPanel(patchpanel.TokenSeparator, patchpanel.KeyValueSeparator)

	if err := populateDefaults(patch, reflect.ValueOf(&configStruct)); err != nil {
		return &Config{}, err
	}
	return &configStruct, nil
}

func main() {
	valuesFile := patchpanel.GetFileEnvOrPath(patchpanel.ENV_CONFIG_FILE, patchpanel.FLAG_CONFIG_FILE)
	conf, err := ParseConfig(valuesFile, Config{})
	if err != nil {
		_, _ = os.Stderr.WriteString(fmt.Sprintf("error parsing configuration: %s\n", err.Error()))
		os.Exit(1)
	}
	fmt.Printf("%+v\n", *conf)

	cfgType := patchpanel.ToReflectType(conf)
	fmt.Println("Config type: ", cfgType)
}
