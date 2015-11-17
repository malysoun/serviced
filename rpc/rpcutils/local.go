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

package rpcutils

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

var (
	localRpcClient = &localClient{}
	localAddrs     = map[string]struct{}{}
)

type localClient struct {
	sync.RWMutex
	rcvrs map[string]interface{}
}

func init() {
	localRpcClient.rcvrs = make(map[string]interface{})
	localAddrs = make(map[string]struct{})
}

func RegisterLocalAddress(addrs ...string) {
	for _, addr := range addrs {
		localAddrs[addr] = struct{}{}
	}
}

func RegisterLocal(name string, rcvr interface{}) error {

	return localRpcClient.register(name, rcvr)

}

func (l *localClient) register(name string, rcvr interface{}) error {
	l.Lock()
	defer l.Unlock()
	l.rcvrs[name] = rcvr
	return nil
}

func (l *localClient) Close() error {
	return nil
}
func (l *localClient) Call(serviceMethod string, args interface{}, reply interface{}, timeout time.Duration) error {

	inputs := make([]reflect.Value, 2)
	inputs[0] = reflect.ValueOf(args)
	inputs[1] = reflect.ValueOf(reply)

	parts := strings.SplitN(serviceMethod, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("Invalid service method: %s", serviceMethod)
	}
	name := parts[0]
	methodName := parts[1]

	l.RLock()
	server := l.rcvrs[name]
	l.RUnlock()

	method := reflect.ValueOf(server).MethodByName(methodName)

	callChan := make(chan error, 1)
	go func() {
		result := method.Call(inputs)
		err := result[0].Interface()
		if err != nil {
			callChan <- err.(error)
		}
		callChan <- nil
	}()
	if timeout <= 0 {
		timeout = 365 * 24 * time.Hour
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case result := <-callChan:
		return result
	case <-t.C:
		return fmt.Errorf("call %s timedout waiting for reply", serviceMethod)
	}
}
