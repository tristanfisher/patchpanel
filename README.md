# patchpanel


[![Go
Reference](https://pkg.go.dev/badge/github.com/tristanfisher/patchpanel.svg)](https://pkg.go.dev/github.com/tristanfisher/patchpanel)
[![Go Report
Card](https://goreportcard.com/badge/github.com/tristanfisher/patchpanel)](https://goreportcard.com/report/github.com/tristanfisher/patchpanel)
[![CI Build Status](https://github.com/tristanfisher/patchpanel/actions/workflows/ci.yaml/badge.svg)](https://github.com/tristanfisher/patchpanel/actions/workflows/ci.yaml)


`patchpanel` provides a thread-safe utility for coercing configuration values into strongly typed values on a struct,
which is especially useful for loading up configuration data or complex arguments for a function.

No external dependencies are used, but `patchpanel` is built to easily integrate with other libraries.

<!-- TOC -->
* [patchpanel](#patchpanel)
    * [Metadata / struct tags](#metadata--struct-tags)
    * [Custom parsers](#custom-parsers)
    * [Functionality](#functionality)
    * [Examples](#examples)
      * [Simple usage:](#simple-usage)
      * [Usage of the `GetDefault()` convenience function:](#usage-of-the-getdefault-convenience-function)
      * [Adding a custom parser and using a default value:](#adding-a-custom-parser-and-using-a-default-value)
      * [Integrating spf13/viper with patchpanel:](#integrating-spf13viper-with-patchpanel)
<!-- TOC -->


### Metadata / struct tags

Support for evaluating metadata (such as default values or specific configuration tags) is built-in for converting
string representations to corresponding native types, performed automatically to the type of the struct field 
to which they are attached (e.g., string, int, bool, time.Duration, time.Time).

Secondary struct tags are also supported for influencing how the primary tag is parsed. For example, a timeFormat tag 
dictates how a default tag representing a timestamp should be interpreted.

### Custom parsers

Consumers can register user-defined types and parsing logic at runtime.

See: `type Parser func(value string, parserHints map[string]any) (any, error)`

and

```
func (pc *PatchPanel) AddParser(typ reflect.Type, parser Parser) {
	pc.Lock()
	defer pc.Unlock()
	pc.parsers[typ] = parser
}
```

when parsing a type, `patchpanel` will call the registered parser (if available) along with any parser hints that are provided.


### Functionality

- Getting values via [struct tags](https://go.dev/ref/spec#Tag)
- Type coercions / deserializers

### Examples

#### Simple usage:

```go
type ServerConfig struct {
    Port int `env:"PORT" default:"8080"`
    TLS  bool `default:"true"`
}

func main() {
    panel := patchpanel.NewPatchPanel(patchpanel.TokenSeparator, patchpanel.KeyValueSeparator)
    cfgType := patchpanel.ToReflectType(ServerConfig{})

    // extract the "default" tag from the "Port" field
    _, val, err := panel.GetFieldTag("Port", "default", cfgType, nil)
    if err != nil {
        panic(err)
    }

    // the Port val is now strongly typed as int(8080)
    fmt.Printf("Type: %T, Value: %v\n", val, val)
}
```

#### Usage of the `GetDefault()` convenience function:

```go
type RateLimit struct {
    MaxRequests int `default:"100"`
}

func main() {
    panel := patchpanel.NewPatchPanel("·", ":")
    rlType := patchpanel.ToReflectType(RateLimit{})

    val, err := panel.GetDefault("MaxRequests", rlType, nil)
    if err != nil {
        // Handle specific errors such as missing values
        if errors.As(err, &patchpanel.NoValueError{}) {
            fmt.Println("No default value provided.")
        }
    }
}
```

#### Adding a custom parser and using a default value:

```go
type IPv4Address string

func main() {
    panel := patchpanel.NewPatchPanel(patchpanel.TokenSeparator, patchpanel.KeyValueSeparator)

    // Define the parser for the custom type IPv4Address
    ipv4Parser := func(v string, hints map[string]any) (any, error) {
        parts := strings.Split(v, ".")
        if len(parts) != 4 {
            return nil, errors.New("invalid ipv4 format")
        }
        return IPv4Address(v), nil
    }

    // Register the parser against the reflect.Type of IPv4Address
    panel.AddParser(patchpanel.ToReflectType(IPv4Address("")), ipv4Parser)

    type NetworkConfig struct {
        BindIP IPv4Address `default:"127.0.0.1"`
    }

    cfgType := patchpanel.ToReflectType(NetworkConfig{})
    val, err := panel.GetDefault("BindIP", cfgType, nil)
    if err != nil {
        panic(err)
    }

    // val is now strongly typed as IPv4Address("127.0.0.1")
}
```

#### Integrating spf13/viper with patchpanel:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"github.com/tristanfisher/patchpanel"
	"os"
	"reflect"
	"time"
)

type Config struct {
	Runtime time.Duration `default:"5s"`
}

func ParseConfig(configPath string, configStruct Config) (*Config, error) {
	viperConf := viper.New()
	patch := patchpanel.NewPatchPanel(patchpanel.TokenSeparator, patchpanel.KeyValueSeparator)

	// get defaults off of our struct using patchpanel
	confType := reflect.TypeOf(configStruct)
	for i := 0; i < confType.NumField(); i++ {
		fieldVal, err := patch.GetDefault(confType.Field(i).Name, confType, []string{})
		if err != nil {
			return &Config{}, err
		}
		viperConf.SetDefault(confType.Field(i).Name, fieldVal)
	}

	// check configuration file for more values
	if configPath != "" {
		viperConf.SetConfigFile(configPath)
		err := viperConf.ReadInConfig()
		if err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if errors.As(err, &configFileNotFoundError) {
				return nil, fmt.Errorf("file not found: %s", err)
			}
			return &Config{}, err
		}
	}

	viperConf.AutomaticEnv()
	err := viperConf.Unmarshal(&configStruct)
	if err != nil {
		return &Config{}, err
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

	_, _  = os.Stdout.WriteString(fmt.Sprintf(":)\n"))
	ctx, cancel := context.WithTimeout(context.Background(), conf.Runtime)
	defer cancel()
	select {
		case <-ctx.Done():
			fmt.Println("have a good day")
	}
}
```
