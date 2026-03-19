   SELECT '
digraph structs {
'
UNION ALL
   SELECT '
rankdir="LR"
'
UNION ALL
   SELECT '
node [shape=none]
'
UNION ALL
   SELECT node_line
     FROM (
           SELECT node_line
             FROM (
                   SELECT CASE
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
                       >];
                       '
                         END AS node_line,
                          t.name AS table_name,
                          i.cid AS column_id
                     FROM pragma_table_list () AS t
                     JOIN pragma_table_info (t.name, t.schema) AS i
                    WHERE t.name NOT LIKE 'sqlite_%'
                      AND t.type = 'table'
                  )
            ORDER BY table_name,
                     column_id
          )
UNION ALL
   SELECT edge_line
     FROM (
           SELECT edge_line
             FROM (
                   SELECT t.name || ':' || f."from" || '_from:e -> ' || CASE
                                   WHEN f."to" IS NULL
                                          OR f."to" = 'rowid' THEN f."table"
                                             ELSE f."table" || ':' || f."to" || '_to:w'
                         END AS edge_line,
                          t.name AS table_name,
                          f.id AS fk_id,
                          f.seq AS fk_seq
                     FROM pragma_table_list () AS t
                     JOIN pragma_foreign_key_list (t.name, t.schema) AS f
                  )
            ORDER BY table_name,
                     fk_id,
                     fk_seq
          )
UNION ALL
   SELECT '
}';
