package signalr

import (
	"errors"
	"strings"
)

type ParsedConnString struct {
	Endpoint string
	Key      string
	Version  string
}

// ParseConnectionString will parse the SignalR connection string from the Azure Portal
func ParseConnectionString(connStr string) (*ParsedConnString, error) {
	connStr = strings.TrimSuffix(connStr, ";")
	splits := strings.Split(connStr, ";")
	parsed := new(ParsedConnString)
	for _, combo := range splits {
		println(combo)
		location := strings.Index(combo, "=") // find the first instance of a "="
		if location == -1 {
			return nil, errors.New("connStr: " + connStr + " did not have a '=' between the ';', so it's malformed.")
		}
		key := combo[0:location]
		value := combo[location+1:]
		if key == "Endpoint" {
			parsed.Endpoint = value
		} else if key == "AccessKey" {
			parsed.Key = value
		} else if key == "Version" {
			parsed.Version = value
		} else {
			return nil, errors.New("unknown key == " + key)
		}
	}
	return parsed, nil
}
