package doodle

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

var doodleLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Keyword", Pattern: `(?i)\b(SELECT|DISTINCT|FROM|WHERE|VERSION|GROUP|HAVING|LIMIT|OFFSET|ORDER|BY|ASC|DESC|AND|OR|IN|NOT|LIKE|BETWEEN|NULL|IS|EXISTS|true|false|AS|COUNT|SUM|AVG|MIN|MAX|UNION|INTERSECT|EXCEPT|ALL|CASE|WHEN|THEN|ELSE|END|COALESCE|NULLIF|WITH|UPPER|LOWER|CONCAT|TRIM|SUBSTRING|LENGTH|DATE_TRUNC|EXTRACT|NOW|CURRENT_DATE|ABS|ROUND|CEIL|FLOOR|ARRAY_AGG|STRING_AGG|JSON_AGG|JSON_BUILD_OBJECT|JSON_GET|JSON_TEXT|JSON_PATH|JSON_PATH_TEXT|INTERVAL|YEAR|MONTH|DAY|HOUR|MINUTE|SECOND|INSERT|INTO|VALUES|SET|DELETE|UPDATE|RETURNING|FORCE)\b`},
	{Name: "DateTime", Pattern: `d'[^']*'`},
	{Name: "String", Pattern: `'[^']*'`},
	{Name: "Float", Pattern: `[-+]?\d+\.\d+`},
	{Name: "Int", Pattern: `[-+]?\d+`},
	{Name: "Arrow", Pattern: `->!|<!-|->\?|<\?-|->|<-`},
	{Name: "Operator", Pattern: `>=|<=|!=|=|>|<`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "Punct", Pattern: `[(),.:*{}+\-]`},
	{Name: "whitespace", Pattern: `\s+`},
})

// Parser parses doodle query syntax
var Parser = participle.MustBuild[Query](
	participle.Lexer(doodleLexer),
	participle.Unquote("String"),
	participle.Map(func(t lexer.Token) (lexer.Token, error) {
		// Strip d'' wrapper from datetime
		if t.Type == doodleLexer.Symbols()["DateTime"] {
			t.Value = t.Value[2 : len(t.Value)-1]
		}
		return t, nil
	}, "DateTime"),
	participle.CaseInsensitive("Keyword"),
	participle.UseLookahead(50),
)

// Parse parses a doodle query string into an AST
func Parse(query string) (*Query, error) {
	return Parser.ParseString("", query)
}

// StatementParser parses any doodle statement (SELECT, INSERT, UPDATE, DELETE)
var StatementParser = participle.MustBuild[Statement](
	participle.Lexer(doodleLexer),
	participle.Unquote("String"),
	participle.Map(func(t lexer.Token) (lexer.Token, error) {
		// Strip d'' wrapper from datetime
		if t.Type == doodleLexer.Symbols()["DateTime"] {
			t.Value = t.Value[2 : len(t.Value)-1]
		}
		return t, nil
	}, "DateTime"),
	participle.CaseInsensitive("Keyword"),
	participle.UseLookahead(50),
)

// ParseStatement parses a doodle statement (SELECT, INSERT, UPDATE, DELETE)
func ParseStatement(stmt string) (*Statement, error) {
	return StatementParser.ParseString("", stmt)
}
