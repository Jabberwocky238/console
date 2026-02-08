这条语句是查询指定db下所有数据表和其数据列所占大小的所有历史快照

```sql
SELECT 
    db.name AS db_name,
	s."statisticID", 
    s."columnIDs",
    sc.name AS schema_name,
    n.name AS table_name,
    (s."rowCount" * s."avgSize")::INT8 AS table_bytes,
    s."rowCount"::INT8 AS est_rows,
    s."createdAt" AS last_updated
FROM 
    system.table_statistics AS s
JOIN 
    system.namespace AS n ON s."tableID" = n.id
JOIN 
    system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN 
    system.namespace AS db ON n."parentID" = db.id
WHERE 
    db.name = 'db_jabber147008'
ORDER BY 
    n.name ASC, last_updated DESC;
```

这条语句是查询指定db下所有数据表和其数据列所占大小，的最新快照

```sql
SELECT 
    db.name AS db_name,
    sc.name AS schema_name,
    n.name AS table_name,
    s."columnIDs",
    (s."rowCount" * s."avgSize")::INT8 AS table_bytes,
    s."rowCount"::INT8 AS est_rows,
    s."createdAt" AS first_snapshot_time,
    s."statisticID"
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
    AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID")
ORDER BY 
    table_name ASC, s."columnIDs" ASC;
```

查询指定数据库的，所有数据表和schema和其大小
```sql
SELECT 
    n.name AS table_name,
    sc.name AS schema_name,
    SUM((s."rowCount" * s."avgSize")::INT8) AS total_bytes,
    MAX(s."createdAt") AS last_updated
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID")
GROUP BY n.name, sc.name
ORDER BY total_bytes DESC;
```


查询指定数据库的，所有schema和其大小
```sql
SELECT 
    sc.name AS schema_name,
    SUM((s."rowCount" * s."avgSize")::INT8) AS total_schema_bytes,
    MAX(s."createdAt") AS last_updated
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID")
GROUP BY sc.name
ORDER BY total_schema_bytes DESC;
```


查询数据库大小
```sql
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS total_bytes
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
```

指定数据库指定schema大小
```sql
SELECT 
    SUM((s."rowCount" * s."avgSize")::INT8) AS schema_bytes,
    MAX(s."createdAt") AS last_updated
FROM system.table_statistics AS s
JOIN system.namespace AS n ON s."tableID" = n.id
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008' 
  AND sc.name = 'schema_303737e93eb57281'
  AND s."createdAt" = (SELECT MAX("createdAt") FROM system.table_statistics WHERE "tableID" = s."tableID");
```

查询数据库schema和表名
```sql 
SELECT 
    sc.name AS schema_name,
    n.name AS table_name
FROM system.namespace AS n
JOIN system.namespace AS sc ON n."parentSchemaID" = sc.id
JOIN system.namespace AS db ON n."parentID" = db.id
WHERE db.name = 'db_jabber147008'
ORDER BY schema_name, table_name;
```