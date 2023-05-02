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
	id := int64(jsonData["Id"].(float64))
	doc, _ := server.findDoc(id)
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
	}
	firstWg.Wait()

	secondWg := new(sync.WaitGroup)
	secondWg.Add(3)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		server.updateDepth(doc, inputChilds)
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
	doc, _ := server.findDoc(id)
	if doc.ChildList != nil {
		for _, value := range doc.ChildList {
			server.innerDelete(value)
		}
	}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func(wg *sync.WaitGroup) {
		server.delFromCache(id)
		wg.Done()
	}(wg)
	go func(wg *sync.WaitGroup) {
		server.delFromDB(id)
		wg.Done()
	}(wg)
	wg.Wait()
	return doc
}

func (server *Server) delFromCache(id int64) {
	server.cache.DelDoc(id)
}

func (server *Server) delFromDB(id int64) {
	server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).Delete()
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

func (server *Server) updateDepth(document *models.Document, newChilds []int64) {
	doc := document
	id := doc.Id
	childs := newChilds
	depth := doc.Depth
	for id != 0 {
		query := server.db.Query(server.config.CollectionName).WhereInt64("id", reindexer.EQ, childs...)
		query.AggregateMax("Depth")
		iterator := query.Exec()
		maxChildDepth := -1
		if len(iterator.AggResults()) != 0 {
			maxChildDepth = int(iterator.AggResults()[0].Value)
		}
		iterator.Close()
		if maxChildDepth+1 == depth {
			break
		}
		server.db.Query(server.config.CollectionName).Where("id", reindexer.EQ, id).
			Set("Depth", maxChildDepth+1).Update()
		id = doc.ParentId
		parentDoc, _ := server.findDoc(id)
		if parentDoc == nil {
			return
		}
		childs = parentDoc.ChildList
		depth = parentDoc.Depth
	}
}
