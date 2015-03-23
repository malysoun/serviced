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

package scheduler

import "github.com/zenoss/glog"

// Synchronizer is an interface type for synchronizing data
type Synchronizer interface {
	// DataMap returns a map of id to object data from the originating db
	DataMap() map[string]interface{}

	// IDMap returns a hashmap to the receiving db
	IDMap() (map[string]struct{}, error)

	// Create creates a new object in the receieving db
	Create(data interface{}) error

	// Update updates an existing object in the receieving db
	Update(data interface{}) error

	// Delete deletes an object in the receiving db
	Delete(id string) error
}

func Sync(s Synchronizer) error {
	ids, err := s.IDMap()
	if err != nil {
		return err
	}

	for id, data := range s.DataMap() {
		if _, ok := ids[id]; ok {
			if err := s.Update(data); err != nil {
				glog.Errorf("Could not update %s: %s", id, err)
				return err
			}
			delete(ids, id)
		} else {
			if err := s.Create(data); err != nil {
				glog.Errorf("Could not create %s: %s", id, err)
				return err
			}
		}
	}

	for _, id := range ids {
		if err := s.Delete(id); err != nil {
			glog.Errorf("Could not delete %s: %s", id, err)
			return err
		}
	}

	return nil
}

func SyncW(s Synchronizer, shutdown <-chan struct{}, conn client.Connection, rootpath string) {
	for {
		ids, ev, err := conn.ChildrenW(rootpath)
		if err != nil {
			glog.Errorf("Could not children of %s: %s", rootpath, err)
			return err
		}

		for _, id := range ids {
			go func(id string) {
				for {
					node := s.Node()
					ev, err := conn.GetW(path.Join(rootpath, id), node)
					if err == client.ErrNoNode {
						if err := s.Delete(node); err != nil {
							glog.Errorf("Could not remove %s: %s", path.Join(rootpath, id), err)
							return err
						}
						return nil
					} else if err != nil {
						glog.Errorf("Could not look up %s: %s", path.Join(rootpath, id), err)
						return err
					} else {
						if err := s.Update(node); err != nil {
							glog.Errorf("Could not update %s: %s", path.Join(rootpath, id), err)
							return err
						}
					}
					select {
					case <-ev:
						// pass
					case <-shutdown:
						return
					}

				}

			}
		}
	}
}
