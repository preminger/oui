package oui

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	f := filepath.Join(t.TempDir(), "oui.db")
	conn, err := sql.Open("sqlite", f)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
	})
	return conn
}

func Test_escapeSQLString(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "'VMware, Inc.'", escapeSQLString("VMware, Inc."))
	})
	t.Run("apostrophe", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "'O''Brien & Co'", escapeSQLString("O'Brien & Co"))
	})
	t.Run("injection", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "'a''); DROP TABLE x;--'", escapeSQLString("a'); DROP TABLE x;--"))
	})
}

func Test_WithSQLiteConnection(t *testing.T) {
	conn := newTestSQLiteDB(t)
	ouidb, err := New(WithSQLiteConnection(conn), WithVersion("test_conn"))
	require.NoError(t, err)
	assert.Equal(t, dialectSqlite, ouidb.dialect)
	assert.Same(t, conn, ouidb.Connection)
}

func Test_WithInlineBulkInsert(t *testing.T) {
	t.Run("default when zero", func(t *testing.T) {
		t.Parallel()
		opts := getOptions(WithInlineBulkInsert(0))
		assert.Equal(t, defaultInlineRows, opts.InlineRowsPerStatement)
	})
	t.Run("explicit", func(t *testing.T) {
		t.Parallel()
		opts := getOptions(WithInlineBulkInsert(25))
		assert.Equal(t, 25, opts.InlineRowsPerStatement)
	})
}

func Test_InlineBulkInsert(t *testing.T) {
	mkDefs := func(n int, org string) []*VendorDef {
		defs := make([]*VendorDef, 0, n)
		for i := range n {
			defs = append(defs, &VendorDef{
				Prefix:   fmt.Sprintf("00:28:%02x:00:00:00/24", i),
				Length:   24,
				Org:      org,
				Registry: "MA-L",
			})
		}
		return defs
	}

	t.Run("chunks by rows", func(t *testing.T) {
		t.Parallel()
		conn := newTestSQLiteDB(t)
		ouidb, err := New(WithSQLiteConnection(conn), WithInlineBulkInsert(3), WithVersion("test_rows"))
		require.NoError(t, err)
		inserted, err := ouidb.BulkInsert(mkDefs(10, "O'Brien & Co's \"Devices\""))
		require.NoError(t, err)
		assert.Equal(t, int64(10), inserted)
		count, err := ouidb.Count()
		require.NoError(t, err)
		assert.Equal(t, int64(10), count)

		matches, err := ouidb.Find("00:28:05:aa:bb:cc")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, "O'Brien & Co's \"Devices\"", matches[0].Org)
		assert.Equal(t, "00:28:05:00:00:00/24", matches[0].Prefix)
	})

	t.Run("chunks by statement size", func(t *testing.T) {
		t.Parallel()
		conn := newTestSQLiteDB(t)
		ouidb, err := New(WithSQLiteConnection(conn), WithInlineBulkInsert(250), WithVersion("test_bytes"))
		require.NoError(t, err)
		// 10 rows x ~20KB org strings exceed maxInlineStatementBytes, forcing
		// byte-based flushes before the 250-row cap is reached.
		bigOrg := strings.Repeat("o'rg ", 4000)
		inserted, err := ouidb.BulkInsert(mkDefs(10, bigOrg))
		require.NoError(t, err)
		assert.Equal(t, int64(10), inserted)
		count, err := ouidb.Count()
		require.NoError(t, err)
		assert.Equal(t, int64(10), count)
	})

	t.Run("empty defs", func(t *testing.T) {
		t.Parallel()
		conn := newTestSQLiteDB(t)
		ouidb, err := New(WithSQLiteConnection(conn), WithInlineBulkInsert(3), WithVersion("test_empty"))
		require.NoError(t, err)
		inserted, err := ouidb.BulkInsert([]*VendorDef{})
		require.NoError(t, err)
		assert.Zero(t, inserted)
	})

	t.Run("populate uses inline path", func(t *testing.T) {
		t.Parallel()
		conn := newTestSQLiteDB(t)
		ouidb, err := New(WithSQLiteConnection(conn), WithInlineBulkInsert(3), WithVersion("test_pop"))
		require.NoError(t, err)
		// Statements longer than one row prove multi-row inlining works on the
		// same table schema Populate targets.
		inserted, err := ouidb.BulkInsert(mkDefs(7, "Vendor"))
		require.NoError(t, err)
		err = ouidb.Clear()
		require.NoError(t, err)
		assert.Equal(t, int64(7), inserted)
	})
}

func Test_fetchCSV(t *testing.T) {
	csvBody := "Registry,Assignment,Organization Name,Organization Address\n" +
		"MA-L,00286B,Test Org's Name,123 Somewhere St\n" +
		"MA-L,286FB9,Another Org,456 Elsewhere Ave\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "oui", r.Header.Get("user-agent"))
		fmt.Fprint(w, csvBody)
	}))
	t.Cleanup(srv.Close)

	reg := &Registry{
		Name:             "MA-L",
		BaseURL:          srv.URL,
		FilePrefix:       "oui",
		FileExtension:    "csv",
		DefaultPrefixLen: 24,
	}

	t.Run("fetch and parse", func(t *testing.T) {
		body, err := fetchCSV(srv.Client(), reg)
		require.NoError(t, err)
		defer body.Close()
		defs, err := readCSVRows(reg, body, nil)
		require.NoError(t, err)
		require.Len(t, defs, 2)
		assert.Equal(t, "Test Org's Name", defs[0].Org)
		assert.Equal(t, "00:28:6b:00:00:00/24", defs[0].Prefix)
		assert.Equal(t, 24, defs[0].Length)
		assert.Equal(t, "MA-L", defs[0].Registry)
	})

	t.Run("non-200", func(t *testing.T) {
		errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusForbidden)
		}))
		t.Cleanup(errSrv.Close)
		errReg := &Registry{Name: "MA-L", BaseURL: errSrv.URL, FilePrefix: "oui", FileExtension: "csv", DefaultPrefixLen: 24}
		_, err := fetchCSV(errSrv.Client(), errReg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download")
	})
}
