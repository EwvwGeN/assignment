package cache

import (
	"sync"
	"time"

	"github.com/EwvwGeN/assignment/internal/models"
)

type status int

const (
	unoccupied status = 0
	working    status = 1
	awaitLock  status = 2
)

const (
	startWork  = 1
	cancelWork = -1
	update     = 0
)

type Cache struct {
	sync.RWMutex
	state            status
	lifeTime         time.Duration
	cleaningInterval time.Duration
	innerAction      chan int
	lockChan         chan chan bool
	docsConroller    map[int64]*extDoc
}

type extDoc struct {
	sync.RWMutex
	expiration int64
	doc        *models.Document
}

func NewCache(lifeTime, cleaningInterval time.Duration) *Cache {
	cache := &Cache{
		state:            unoccupied,
		lifeTime:         lifeTime,
		cleaningInterval: cleaningInterval,
		innerAction:      make(chan int),
		lockChan:         make(chan chan bool),
		docsConroller:    make(map[int64]*extDoc),
	}

	if cleaningInterval > 0 {
		go cache.garbageCollector()
	}

	go cache.controller()

	return cache
}

func (cache *Cache) controller() {
	var chans []chan bool
	cChan := make(chan bool)
	go func() {
		usageCount := 0
		for {
			usageCount += <-cache.innerAction
			if usageCount != 0 && cache.state == awaitLock {
				continue
			}
			if usageCount == 0 && cache.state != awaitLock {
				cache.state = unoccupied
				continue
			}
			if usageCount == 0 && cache.state == awaitLock {
				cChan <- true
				<-cChan
				if len(chans) == 0 {
					cache.state = unoccupied
				}
				continue
			}
			cache.state = working
		}
	}()
	go func() {
		go func() {
			for {
				chans = append(chans, <-cache.lockChan)
				cache.state = awaitLock
				cache.innerAction <- update
			}
		}()
		go func() {
			for {
				<-cChan
				for len(chans) != 0 {
					chans[0] <- true
					<-chans[0]
					chans = chans[1:]
				}
				cChan <- true
			}
		}()
	}()
}

func (cache *Cache) garbageCollector() {
	for {
		<-time.After(cache.cleaningInterval)
		if cache.docsConroller == nil {
			return
		}
		if keys := cache.checkExpired(); len(keys) != 0 {
			cache.clearExpired(keys)
		}
	}
}

func (cache *Cache) checkExpired() (keys []int64) {
	keys = []int64{}
	waiter := make(chan bool)
	cache.lockChan <- waiter
	<-waiter
	cache.RLock()
	for i, dc := range cache.docsConroller {
		if time.Now().UnixNano() > dc.expiration && dc.expiration > 0 {
			keys = append(keys, i)
		}
	}
	cache.RUnlock()
	waiter <- true
	return keys
}

func (cache *Cache) clearExpired(keys []int64) {
	waiter := make(chan bool)
	cache.lockChan <- waiter
	<-waiter
	cache.RLock()
	for _, i := range keys {
		delete(cache.docsConroller, i)
	}
	cache.RUnlock()
	waiter <- true
}

func (cache *Cache) checkExist(id int64) bool {
	if cache.docsConroller[id] == nil {
		return false
	}
	return true
}

func (cache *Cache) AddDoc(doc *models.Document) {
	cache.innerAddDoc(doc)
}

func (cache *Cache) innerAddDoc(doc *models.Document) {
	id := doc.Id
	cache.docsConroller[id] = &extDoc{
		expiration: time.Now().Add(cache.lifeTime).UnixNano(),
		doc:        doc,
	}
}

func (cache *Cache) DelDoc(id int64) {
	cache.innerDelDoc(id)
}

func (cache *Cache) innerDelDoc(id int64) {
	if !cache.checkExist(id) {
		return
	}
	cache.docsConroller[id].Lock()
	cache.docsConroller[id].Unlock()
	delete(cache.docsConroller, id)
}

func (cache *Cache) GetDoc(id int64) *models.Document {
	return cache.innerGetDoc(id)
}

func (cache *Cache) innerGetDoc(id int64) *models.Document {
	if !cache.checkExist(id) {
		return nil
	}
	for cache.state == awaitLock {
	}
	cache.innerAction <- startWork
	cache.docsConroller[id].Lock()
	if !cache.checkExist(id) {
		return nil
	}
	buffer := cache.docsConroller[id]
	cache.docsConroller[id].expiration = time.Now().Add(cache.lifeTime).UnixNano()
	cache.docsConroller[id].Unlock()
	cache.innerAction <- cancelWork
	return buffer.doc
}

func (cache *Cache) UpdateDoc(doc *models.Document) {
	cache.innerUpdateDoc(doc)
}

func (cache *Cache) innerUpdateDoc(doc *models.Document) {
	id := doc.Id
	if !cache.checkExist(id) {
		cache.innerAddDoc(doc)
		return
	}
	for cache.state == awaitLock {
	}
	cache.innerAction <- startWork
	cache.docsConroller[id].RLock()
	cache.docsConroller[id].doc = doc
	cache.docsConroller[id].expiration = time.Now().Add(cache.lifeTime).UnixNano()
	cache.docsConroller[id].RUnlock()
	cache.innerAction <- cancelWork
}
