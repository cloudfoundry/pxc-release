package thermostat

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Properties `yaml:"properties"`
	IP         string `yaml:"ip"`
}

func LoadConfig() (*Config, error) {
	path := os.Getenv("CONFIG")
	if len(path) == 0 {
		return nil, errors.New("CONFIG environment variable must be set")
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

type Properties map[interface{}]interface{}

func (p Properties) Find(lens string) (val interface{}, err error) {
	matchers := strings.Split(lens, ".")

	if len(matchers) == 1 {
		val, found := p[matchers[0]]
		if !found {
			return nil, errors.New("value not found")
		}
		return val, nil
	}

	m := matchers[0]

	if next, present := p[m]; present {
		n, ok := next.(Properties)
		if !ok {
			panic("type conversion failed")
		}
		return n.Find(strings.Join(matchers[1:], "."))
	} else {
		return nil, errors.New("value not found")
	}
}

func (p Properties) FindString(lens string) (val string, err error) {
	s, err := p.Find(lens)
	if err != nil {
		return "", err
	}

	val, ok := s.(string)
	if !ok {
		return "", errors.New("value not a string")
	}

	return val, nil
}

func (p Properties) FindBool(lens string) (val bool, err error) {
	b, err := p.Find(lens)
	if err != nil {
		return false, err
	}

	val, ok := b.(bool)
	if !ok {
		return false, errors.New("value not a boolean")
	}

	return val, nil
}
