package oui

type VendorDef struct {
	Prefix   string
	Length   int
	Org      string
	Registry string
}

func (v *VendorDef) PrefixString() string {
	if v == nil {
		return "<nil>"
	}
	return v.Prefix
}

type LoggerType interface {
	Success(s string, f ...any)
	Info(s string, f ...any)
	Warn(s string, f ...any)
	Error(s string, f ...any)
	Err(err error, strs ...string)
}

const (
	dialectSqlite int = iota
	dialectPsql
)

const (
	maxVarsSqlite int = 999
	maxVarsPsql   int = 65535
)

const (
	// Hosted SQLite services such as Cloudflare D1 cap statement size at 100KB;
	// stay safely under it when inlining values.
	maxInlineStatementBytes int = 95_000
	defaultInlineRows       int = 250
)
