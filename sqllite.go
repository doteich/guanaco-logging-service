package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type payload struct {
	id       int
	ts       time.Time
	nodeName string
	nodeId   string
	dataType string
	value    string
}

var DB *sql.DB

func InitDB(ctx context.Context, n string, d string) error {

	var err error
	_, err = os.Stat("./sqlite")

	if err != nil {
		os.Mkdir("./sqlite", 0755)
	}

	DB, err = sql.Open("sqlite", "./sqlite/"+n+".db")

	if err != nil {
		return err

	}

	_, err = DB.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS guanaco (
			id INTEGER PRIMARY KEY AUTOINCREMENT, 
			ts DATETIME NOT NULL, 
			nodeName TEXT NOT NULL, 
			nodeId TEXT NOT NULL,
			dataType TEXT NOT NULL,
			value TEXT NOT NULL
		)`,
	)
	if err != nil {
		return err
	}

	return nil
}

func (p *payload) InsertData(ctx context.Context) {
	_, err := DB.ExecContext(ctx, `INSERT INTO guanaco (ts, nodeName, nodeId, dataType, value)`, p.ts, p.nodeName, p.nodeId, p.dataType, p.value)

	if err != nil {
		Logger.Error(fmt.Sprintf("error while inserting payload to database %s", err.Error()))
	}

}
