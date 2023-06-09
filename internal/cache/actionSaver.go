package cache

import "sync"

type Action string

const (
	DELETE Action = "delete"
	UPDATE Action = "update"
)

//i think this way to transfer action is better then map[id]map[action]map[field]newValue
// or map[id]map[action][]properties{field, newValue}
//
// DocId: identifier of the document
//
// Action: processed action (update or delete)
//
// Field: changeable field
//
// NewValue: new value for field
type ActionProperties struct {
	DocId    int64
	Action   Action
	Field    string
	NewValue interface{}
}

type docActionSaver struct {
	Channel       chan *ActionProperties
	actionStorage map[int64]map[Action]map[string]interface{}
	workingСache  *Cache
}

func (cache *Cache) NewActionSaver() *docActionSaver {
	newSaver := &docActionSaver{
		Channel:       make(chan *ActionProperties),
		actionStorage: make(map[int64]map[Action]map[string]interface{}),
		workingСache:  cache,
	}
	newSaver.controller()
	return newSaver
}

func (das *docActionSaver) controller() {
	go func() {
		for {
			input := <-das.Channel
			if input == nil {
				continue
			}
			if input.Action == "" {
				continue
			}
			if das.actionStorage[input.DocId] == nil {
				das.actionStorage[input.DocId] = map[Action]map[string]interface{}{
					input.Action: {
						input.Field: input.NewValue,
					},
				}
				continue
			}
			switch input.Action {
			case DELETE:
				das.actionStorage[input.DocId] = map[Action]map[string]interface{}{
					DELETE: nil,
				}
			case UPDATE:
				das.actionStorage[input.DocId][input.Action][input.Field] = input.NewValue
			}
		}
	}()
}

func (das *docActionSaver) Rollback() {
	das = nil
}

func (das *docActionSaver) Commit() {
	das.innerCommit()
}

func (das *docActionSaver) innerCommit() {
	wg := new(sync.WaitGroup)
	wg.Add(len(das.actionStorage))
	for id, v := range das.actionStorage {
		go func(id int64, inputMap map[Action]map[string]interface{}, wg *sync.WaitGroup) {
			for action, properties := range inputMap {
				switch action {
				case DELETE:
					das.workingСache.innerDelDoc(id)
				case UPDATE:
					das.workingСache.innerUpdateDoc(id, properties)
				}
			}
			wg.Done()
		}(id, v, wg)
	}
	wg.Wait()
}
