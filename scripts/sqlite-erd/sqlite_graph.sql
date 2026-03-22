WITH rows AS (
       SELECT 0 AS section_sort,
              '' AS table_name,
              0 AS item_sort_1,
              0 AS item_sort_2,
              'digraph structs {' AS line
       UNION ALL
       SELECT 1,
              '',
              0,
              0,
              'rankdir="LR"'
       UNION ALL
       SELECT 2,
              '',
              0,
              0,
              'node [shape=none]'
       UNION ALL
       SELECT 10 AS section_sort,
              t.name AS table_name,
              i.cid AS item_sort_1,
              0 AS item_sort_2,
              CASE
                  WHEN LAG(t.name, 1) OVER (
                       ORDER BY t.name, i.cid
                  ) = t.name THEN ''
                  ELSE t.name || ' [label=<
            <TABLE BORDER="0" CELLSPACING="0" CELLBORDER="1">
                <TR>
                    <TD COLSPAN="2"><B>' || t.name || '</B></TD>
                </TR>
            '
              END || '
                <TR>
                    <TD PORT="' || i.name || '_to">' || CASE i.pk
                    WHEN 0 THEN '&nbsp;'
                    ELSE '🔑'
              END || '</TD>
                    <TD PORT="' || i.name || '_from">' || i.name || '</TD>
                </TR>
            ' || CASE
                    WHEN LEAD(t.name, 1) OVER (
                         ORDER BY t.name, i.cid
                    ) = t.name THEN ''
                    ELSE '
            </TABLE>
        >];'
              END AS line
         FROM pragma_table_list () AS t
         JOIN pragma_table_info (t.name, t.schema) AS i
        WHERE t.name NOT LIKE 'sqlite_%'
          AND t.type = 'table'
       UNION ALL
       SELECT 20 AS section_sort,
              t.name AS table_name,
              f.id AS item_sort_1,
              f.seq AS item_sort_2,
              t.name || ':' || f."from" || '_from:e -> ' || CASE
                  WHEN f."to" IS NULL
                       OR f."to" = 'rowid' THEN f."table"
                  ELSE f."table" || ':' || f."to" || '_to:w'
              END AS line
         FROM pragma_table_list () AS t
         JOIN pragma_foreign_key_list (t.name, t.schema) AS f
        WHERE t.name NOT LIKE 'sqlite_%'
          AND t.type = 'table'
       UNION ALL
       SELECT 30,
              '',
              0,
              0,
              '}'
)
SELECT line
  FROM rows
 ORDER BY section_sort,
          table_name,
          item_sort_1,
          item_sort_2;
