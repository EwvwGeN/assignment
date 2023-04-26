package server

import (
	"fmt"
	"sync"

	"github.com/EwvwGeN/assignment/internal/models"
	"github.com/EwvwGeN/assignment/internal/util"
	"github.com/restream/reindexer"
	"golang.org/x/sync/errgroup"
)

func (server *Server) updateChild(jsonData map[string]interface{}) error {
	if jsonData["ChildList"] == nil {
		return nil
	}
	id := jsonData["Id"].(int64)
	interfaceDoc, _ := server.findDoc(id)
	doc := interfaceDoc
	docChilds := doc.ChildList
	inputChilds := util.ArrToInt64(jsonData["ChildList"].([]interface{}))
	delChilds, addChilds := util.Difference(docChilds, inputChilds)
	if err := server.checkChild(id, addChilds); err != nil {
		return fmt.Errorf("Can not add childs: %w", err)
	}

	firstWg := new(sync.WaitGroup)
	firstWg.Add(len(delChilds))
	for _, childId := range delChilds {
		go func(wg *sync.WaitGroup, childId int64) {
			defer wg.Done()
			server.innerDelete(childId)
		}(firstWg, childId)
		server.innerDelete(childId)
	}
	firstWg.Wait()

	secondWg := new(sync.WaitGroup)
	secondWg.Add(3)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		query := server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, inputChilds...)
		query.AggregateMax("Depth")
		iterator := query.Exec()
		maxChildDepth := int(iterator.AggResults()[0].Value)
		iterator.Close()
		currentId := id
		currentDoc := doc
		for i := 1; currentId != 0; i++ {
			if currentDoc.Depth == maxChildDepth+i {
				break
			}
			server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, currentId).
				Set("Depth", maxChildDepth+i).Update()
			buffer, _ := server.findDoc(currentId)
			currentDoc = buffer
			currentId = currentDoc.ParentId
		}
	}(secondWg)

	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, addChilds...).Set("ParentId", id).Update()
	}(secondWg)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).Set("ChildList", inputChilds).Update()
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

func (server *Server) innerDelete(id int64) *models.Document {
	interfaceDoc, _ := server.findDoc(id)
	doc := interfaceDoc
	if doc.ChildList != nil {
		for _, value := range doc.ChildList {
			server.innerDelete(value)
		}
	}
	server.db.Delete(server.config.CollectionName, doc)
	return doc
}

func (server *Server) findDoc(id int64) (*models.Document, bool) {
	doc := server.getFromCache(id)
	if doc != nil {
		return doc, true
	}
	return server.getFromBD(id)
}

func (server *Server) getFromCache(id int64) *models.Document {
	return server.cache.GetDoc(id)
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
