// Copyright 2015 The Serviced Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const buffersize = 1024

type ParseError string

func (err ParseError) Error() string {
	return fmt.Sprintf("could not parse line: %s", err)
}

type ConfigReader interface {
	StringVal(key, dflt string) string
	StringSlice(key string, dflt []string) []string
	IntVal(key string, dflt int) int
	BoolVal(key string, dflt bool) bool
}

type EnvironConfigReader struct {
	prefix string
}

func NewEnvironConfigReader(filename, prefix string) (*EnvironConfigReader, error) {
	r := &EnvironConfigReader{prefix}

	file, err := os.Open(filename)
	if err != nil {
		return r, err
	}
	defer file.Close()
	if err := r.parse(file); err != nil {
		return r, err
	}
	return r, nil
}

// parse is a really dumb reader parser.  It maps only key values in the form
// of key=value and strips whitespaces surrounding either field.  If the format
// does not match, then and error will return.
func (p *EnvironConfigReader) parse(reader io.Reader) error {
	var (
		line        []byte
		err         error
		isCommented bool = false
		n           int  = buffersize
	)

	for err != io.EOF {
		buffer := make([]byte, buffersize)
		n, err = reader.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}

		// Get the lines and strip out the whitespace
		for _, b := range buffer[:n] {
			if isCommented = isCommented && b != '\n'; !isCommented {
				isCommented = b == '#'

				if b == '#' || b == '\n' {
					if err := keyvalue(line); err != nil {
						return err
					}
					line = make([]byte, 0)
				} else {
					line = append(line, b)
				}
			}
		}
	}

	if err := keyvalue(line); err != nil {
		return err
	}

	return nil
}

func (p *EnvironConfigReader) StringVal(name string, defaultval string) string {
	if val := os.Getenv(p.prefix + name); val != "" {
		return val
	} else {
		return defaultval
	}
}

func (p *EnvironConfigReader) StringSlice(name string, defaultval []string) []string {
	strval := p.StringVal(name, "")
	if strval != "" {
		return strings.Split(strval, ",")
	}
	return defaultval
}

func (p *EnvironConfigReader) IntVal(name string, defaultval int) int {
	strval := p.StringVal(name, "")
	if strval != "" {
		if val, err := strconv.Atoi(strval); err == nil {
			return val
		}
	}
	return defaultval
}

func (p *EnvironConfigReader) BoolVal(name string, defaultval bool) bool {
	strval := p.StringVal(name, "")
	if strval != "" {
		val := strings.ToLower(strval)

		trues := []string{"1", "true", "t", "yes"}
		for _, t := range trues {
			if val == t {
				return true
			}
		}

		falses := []string{"0", "false", "f", "no"}
		for _, f := range falses {
			if val == f {
				return false
			}
		}
	}
	return defaultval
}

func keyvalue(line []byte) error {
	pair := string(line)
	if idx := strings.Index(pair, "="); idx >= 0 {
		key, value := strings.TrimSpace(pair[:idx]), translate(strings.TrimSpace(pair[idx+1:]))
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	} else if pair != "" {
		return ParseError(pair)
	}
	return nil
}

func translate(value string) string {
	result, err := exec.Command("/bin/sh", "-c", fmt.Sprintf("echo -n \"%s\"", value)).Output()
	if err != nil {
		return ""
	}
	return string(result)
}