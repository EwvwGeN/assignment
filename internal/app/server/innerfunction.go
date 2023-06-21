package server

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/EwvwGeN/assignment/internal/cache"
	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/EwvwGeN/assignment/internal/util"
	"github.com/restream/reindexer"
	"golang.org/x/sync/errgroup"
)

func (server *Server) updateChild(tx *reindexer.Tx, channel chan *cache.ActionProperties, jsonData map[string]interface{}) error {
	if jsonData["ChildList"] == nil {
		return nil
	}
	id := jsonData["Id"].(int64)
	doc, _ := server.findDoc(id)
	docChilds := doc.ChildList
	inputChilds := util.ArrToInt64(jsonData["ChildList"].([]interface{}))
	// Splitting the list of child documents into a list for deletion and addition
	delChilds, addChilds := util.Difference(docChilds, inputChilds)
	// Сhecking new documents for the possibility to add them
	if err := server.checkChild(id, addChilds); err != nil {
		return fmt.Errorf("Can not add childs: %w", err)
	}

	firstWg := new(sync.WaitGroup)
	firstWg.Add(len(delChilds))
	for _, childId := range delChilds {
		go func(wg *sync.WaitGroup, childId int64) {
			server.innerDelete(tx, channel, childId)
			wg.Done()
		}(firstWg, childId)
	}
	firstWg.Wait()

	secondWg := new(sync.WaitGroup)
	secondWg.Add(2)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.updateDepth(tx, channel, doc, inputChilds)
	}(secondWg)

	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		for _, v := range addChilds {
			server.innerUpdateFields(tx, channel, v, map[string]interface{}{
				"ParentId": id,
			})
		}
	}(secondWg)

	secondWg.Wait()
	return nil
}

func (server *Server) checkChild(id int64, child []int64) error {
	height, err := server.getDocHeight(id)
	if err != nil {
		return err
	}
	docHeight := height.(int)
	eg := &errgroup.Group{}
	for _, value := range child {
		id := value
		eg.Go(func() error {
			doc, found := server.findDoc(id)
			if !found {
				return fmt.Errorf("%s: File Id:%d", DocumentNotExist.Error(), id)
			}
			buffer := doc
			parentId := buffer.ParentId
			if parentId != 0 {
				return fmt.Errorf("%s: File Id:%d", DocumentHaveParent.Error(), id)
			}
			depth := buffer.Depth
			if depth+docHeight+1 > server.config.NestingLevel {
				return fmt.Errorf("%s: File Id:%d", DeplthLevel.Error(), id)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}

func (server *Server) getDocHeight(id int64) (interface{}, error) {
	var currentHight int
	if id == 0 {
		return 0, nil
	}
	doc, found := server.findDoc(id)
	if !found {
		return nil, fmt.Errorf("%s: File Id:%d", DocumentNotExist.Error(), id)
	}
	document := doc
	for document.ParentId != 0 {
		currentHight++
		doc, _ = server.findDoc(document.ParentId)
		document = doc
	}
	return currentHight, nil
}

func (server *Server) innerDelete(tx *reindexer.Tx, channel chan *cache.ActionProperties, id int64) {
	doc, _ := server.txGetFromDB(tx, id)
	if doc.ChildList != nil {
		for _, value := range doc.ChildList {
			server.innerDelete(tx, channel, value)
		}
	}
	server.txDelFromDB(tx, id)
	channel <- &cache.ActionProperties{
		DocId:  id,
		Action: cache.DELETE,
	}
}

func (server *Server) delFromCache(id int64) {
	server.cache.DelDoc(id)
}

func (server *Server) txDelFromDB(tx *reindexer.Tx, id int64) {
	tx.Query().WhereInt64("id", reindexer.EQ, id).Delete()
}

func (server *Server) delFromDB(id int64) {
	server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).Delete()
}

func (server *Server) findDoc(id int64) (*models.Document, bool) {
	doc := server.getFromCache(id)
	if doc != nil {
		return doc, true
	}
	doc, found := server.getFromBD(id)
	if found {
		server.cache.AddDoc(doc)
	}
	return doc, found
}

func (server *Server) getFromCache(id int64) *models.Document {
	return server.cache.GetDoc(id)
}

func (server *Server) txGetFromDB(tx *reindexer.Tx, id int64) (*models.Document, bool) {
	doc, found := tx.Query().WhereInt64("id", reindexer.EQ, id).Get()
	if !found {
		return nil, found
	}
	return doc.(*models.Document), found
}

func (server *Server) getFromBD(id int64) (*models.Document, bool) {
	query := server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id)
	doc, found := query.Get()
	if !found {
		return nil, found
	}
	return doc.(*models.Document), found
}

func (server *Server) bigDoc(input interface{}) models.BigDocument {
	var bigDoc models.BigDocument
	item := input.(*models.Document)
	bigDoc.Id = item.Id
	bigDoc.Body = item.Body
	for _, childId := range item.ChildList {
		childDoc, _ := server.findDoc(childId)
		bigDoc.ChildList = append(bigDoc.ChildList, server.bigDoc(childDoc))
	}
	return bigDoc
}

func (server *Server) updateDepth(tx *reindexer.Tx, channel chan *cache.ActionProperties, document *models.Document, newChilds []int64) {
	doc := document
	id := doc.Id
	childs := newChilds
	depth := doc.Depth
	previousDepth := -1
	maxChildDepth := -1
	for id != 0 {
		if len(childs) != 0 {
			query := tx.Query().WhereInt64("id", reindexer.EQ, childs...)
			query.AggregateMax("Depth")
			iterator := query.Exec()
			fmt.Println(iterator.AggResults())
			if len(iterator.AggResults()) != 0 {
				maxChildDepth = int(iterator.AggResults()[0].Value)
			}
			iterator.Close()
		}
		if previousDepth > maxChildDepth {
			maxChildDepth = previousDepth
		}
		if maxChildDepth+1 == depth {
			break
		}
		server.innerUpdateFields(tx, channel, id, map[string]interface{}{
			"Depth": maxChildDepth + 1,
		})
		previousDepth = maxChildDepth + 1
		processedСhild := id
		id = doc.ParentId
		parentDoc, found := server.txGetFromDB(tx, id)
		if !found {
			break
		}
		doc = parentDoc
		childs = func() []int64 {
			buffer := doc.ChildList
			for i, value := range buffer {
				if value == processedСhild {
					return append(buffer[:i], buffer[i+1:]...)
				}
			}
			return nil
		}()
		depth = doc.Depth
	}
}

func (server *Server) updateDocFields(tx *reindexer.Tx, channel chan *cache.ActionProperties, id int64, jsonData map[string]interface{}) error {
	changedFields := make(map[string]interface{})
	var document models.AllowedField
	types := reflect.TypeOf(document)
	// Checking the fields for the possibility of changing and saving them in the map
	for key, value := range jsonData {
		if field, exist := types.FieldByName(key); exist {
			changedFields[field.Name] = value
		}
	}
	return server.innerUpdateFields(tx, channel, id, changedFields)
}

// Updating document fields in a transaction and adding action to the saver for future cache update
func (server *Server) innerUpdateFields(tx *reindexer.Tx, channel chan *cache.ActionProperties, id int64, jsonData map[string]interface{}) error {
	var document models.Document
	query := tx.Query().WhereInt64("id", reindexer.EQ, id)
	types := reflect.TypeOf(document)
	for key, value := range jsonData {
		field, _ := types.FieldByName(key)
		query.Set(field.Name, value)
		channel <- &cache.ActionProperties{
			DocId:    id,
			Action:   cache.UPDATE,
			Field:    field.Name,
			NewValue: value,
		}
	}
	query.Update()
	return nil
}
