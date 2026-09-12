// Driver-backed option constructors. These open real database connections
// via drivers that do not compile for js/wasm (e.g. Cloudflare Workers);
// wasm consumers should inject a connection with WithSQLiteConnection instead.

//go:build !js

package oui

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/lib/pq"
	"github.com/preminger/oui/v2/internal/util"
	_ "modernc.org/sqlite"
)

func getFileName() (fn string, err error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	fn = filepath.Join(dir, "oui", "oui.db")
	return
}

func scaffold() (dbf *os.File, dn string, err error) {
	fn, err := getFileName()
	if err != nil {
		return
	}
	dn = filepath.Dir(fn)

	err = os.RemoveAll(dn)
	if err != nil {
		return
	}
	err = os.MkdirAll(dn, 0755)
	if err != nil {
		return
	}
	defer dbf.Close()
	dbf, err = os.Create(fn)
	if err != nil {
		return
	}
	return
}

func CreateSQLiteOption(optionalFileName ...string) (Option, error) {
	var fileName string
	if len(optionalFileName) != 0 {
		fileName = optionalFileName[0]
	} else {
		defaultFileName, err := getFileName()
		if err != nil {
			return nil, err
		}
		fileName = defaultFileName
	}

	var conn *sql.DB

	if !util.PathExists(fileName) {
		_, _, err := scaffold()
		if err != nil {
			return nil, err
		}
		_conn, err := sql.Open("sqlite", fileName)
		if err != nil {
			return nil, err
		}
		conn = _conn
	} else {
		_conn, err := sql.Open("sqlite", fileName)
		if err != nil {
			return nil, err
		}
		conn = _conn
	}
	opt := func(opts *Options) {
		opts.Connection = conn
		opts.dialect = dialectSqlite
	}
	return opt, nil
}

func CreatePostgresOption(connectionString string) (Option, error) {
	conn, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	opt := func(opts *Options) {
		opts.Connection = conn
		opts.dialect = dialectPsql
	}
	return opt, nil
}
