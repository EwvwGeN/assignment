package main

import (
	"fmt"

	"github.com/restream/reindexer"
)

func main() {
	db := reindexer.NewReindex("cproto://user:pass@127.0.0.1:6534/testdb", reindexer.WithCreateDBIfMissing())
	fmt.Println(db.Ping())
}
