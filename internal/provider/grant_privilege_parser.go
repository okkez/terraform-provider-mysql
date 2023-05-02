package provider

import (
	"strings"

	"github.com/pingcap/tidb/parser"
	"github.com/pingcap/tidb/parser/ast"
	_ "github.com/pingcap/tidb/parser/test_driver"
)

type GrantPrivilege struct {
	DBName      string
	TableName   string
	Username    string
	Hostname    string
	Privileges  []*ast.PrivElem
	GrantOption bool
}

func (v *GrantPrivilege) Enter(in ast.Node) (ast.Node, bool) {
	if g, ok := in.(*ast.GrantStmt); ok {
		us := g.Users[0]
		v.Username = us.User.Username
		v.Hostname = us.User.Hostname
		if len(g.Level.DBName) == 0 {
			v.DBName = "*"
		} else {
			v.DBName = g.Level.DBName
		}
		if len(g.Level.TableName) == 0 {
			v.TableName = "*"
		} else {
			v.TableName = g.Level.TableName
		}
		v.GrantOption = g.WithGrant
	}
	if priv, ok := in.(*ast.PrivElem); ok {
		v.Privileges = append(v.Privileges, priv)
	}
	return in, false
}

func (v *GrantPrivilege) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

func (v *GrantPrivilege) PrivString() string {
	if len(v.Privileges) == 0 {
		return ""
	}
	privs := make([]string, len(v.Privileges))
	for _, priv := range v.Privileges {
		if len(priv.Cols) > 0 {
			s := priv.Priv.String()
			columnNames := make([]string, len(priv.Cols))
			for _, col := range priv.Cols {
				columnNames = append(columnNames, col.Name.O)
			}
			privs = append(privs, s+" "+strings.Join(columnNames, ","))
		} else {
			privs = append(privs, priv.Priv.String())
		}
	}

	return strings.Join(privs, ",")
}

func (v *GrantPrivilege) Match(dbName, tableName, username, hostname string) bool {
	return v.DBName == dbName &&
		v.TableName == tableName &&
		v.Username == username &&
		v.Hostname == hostname
}

func extract(rootNode *ast.StmtNode) *GrantPrivilege {
	v := &GrantPrivilege{}
	(*rootNode).Accept(v)
	return v
}

func parse(sql string) (*ast.StmtNode, error) {
	p := parser.New()

	stmtNodes, _, err := p.Parse(sql, "", "")
	if err != nil {
		return nil, err
	}

	return &stmtNodes[0], nil
}

func ParseGrantPrivilegeStatement(sql string) (*GrantPrivilege, error) {
	astNode, err := parse(sql)
	if err != nil {
		return nil, err
	}

	return extract(astNode), nil
}
