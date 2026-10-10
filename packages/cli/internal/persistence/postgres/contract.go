package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"maps"
)

//go:embed schema/contract.json
var expectedContract []byte

// Inspect describes actual physical objects, including constraints, indexes,
// views and unexpected triggers/functions. It never writes or repairs storage.
func Inspect(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `WITH namespaces AS (SELECT oid,nspname FROM pg_namespace WHERE nspname IN ('tokeninsights_data','tokeninsights_accounts'))
 SELECT 'column:'||n.nspname||'.'||c.relname||'.'||a.attnum, a.attname||':'||format_type(a.atttypid,a.atttypmod)||':'||a.attnotnull::text||':'||COALESCE(pg_get_expr(d.adbin,d.adrelid),'')||':'||COALESCE(co.collname,'')
 FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid JOIN pg_attribute a ON a.attrelid=c.oid LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum LEFT JOIN pg_collation co ON co.oid=a.attcollation WHERE c.relkind IN ('r','v') AND a.attnum>0 AND NOT a.attisdropped
 UNION ALL SELECT 'relation:'||n.nspname||'.'||c.relname,c.relkind::text||':'||c.relpersistence::text||':'||c.relrowsecurity::text||':'||c.relforcerowsecurity::text FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid
 UNION ALL SELECT 'constraint:'||n.nspname||'.'||c.relname||'.'||con.conname,pg_get_constraintdef(con.oid)||':'||con.convalidated::text FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid JOIN pg_constraint con ON con.conrelid=c.oid
 UNION ALL SELECT 'index:'||n.nspname||'.'||c.relname,pg_get_indexdef(c.oid)||':'||i.indisvalid::text FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid JOIN pg_index i ON i.indexrelid=c.oid
 UNION ALL SELECT 'view:'||n.nspname||'.'||c.relname,pg_get_viewdef(c.oid) FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid WHERE c.relkind='v'
 UNION ALL SELECT 'trigger:'||n.nspname||'.'||c.relname||'.'||t.tgname,pg_get_triggerdef(t.oid) FROM namespaces n JOIN pg_class c ON c.relnamespace=n.oid JOIN pg_trigger t ON t.tgrelid=c.oid WHERE NOT t.tgisinternal
 UNION ALL SELECT 'function:'||n.nspname||'.'||p.proname||':'||p.oid::text,p.prosrc FROM namespaces n JOIN pg_proc p ON p.pronamespace=n.oid`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	objects := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		objects[key] = value
	}
	return objects, rows.Err()
}

func (s *Store) validate(ctx context.Context) error {
	var expected map[string]string
	if err := json.Unmarshal(expectedContract, &expected); err != nil {
		return err
	}
	actual, err := Inspect(ctx, s.Reader)
	if err != nil {
		return errors.New("incompatible_postgres_schema")
	}
	if !maps.Equal(expected, actual) {
		return errors.New("incompatible_postgres_schema")
	}
	return s.Ready(ctx)
}
