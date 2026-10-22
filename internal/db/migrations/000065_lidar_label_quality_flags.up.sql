-- D-31: split and merge are quality flags, never object classes.
-- Move only the misplaced verdict; retain all existing flags and provenance.
   UPDATE lidar_run_tracks
      SET quality_label = CASE
                    WHEN INSTR(
                    ',' || REPLACE(
                    REPLACE(
                    REPLACE(REPLACE(COALESCE(quality_label, ''), ' ', ''), CHAR(9), '')
                  , CHAR(10)
                  , ''
                    )
                  , CHAR(13)
                  , ''
                    ) || ','
                  , ',' || TRIM(
                    REPLACE(
                    REPLACE(REPLACE(user_label, CHAR(9), ' '), CHAR(10), ' ')
                  , CHAR(13)
                  , ' '
                    )
                    ) || ','
                    ) > 0 THEN quality_label
                    WHEN TRIM(COALESCE(quality_label, '')) = '' THEN TRIM(
                    REPLACE(
                    REPLACE(REPLACE(user_label, CHAR(9), ' '), CHAR(10), ' ')
                  , CHAR(13)
                  , ' '
                    )
                    )
                    ELSE RTRIM(quality_label, ', ') || ',' || TRIM(
                    REPLACE(
                    REPLACE(REPLACE(user_label, CHAR(9), ' '), CHAR(10), ' ')
                  , CHAR(13)
                  , ' '
                    )
                    )
          END
        , user_label = NULL
    WHERE TRIM(
          REPLACE(
          REPLACE(REPLACE(user_label, CHAR(9), ' '), CHAR(10), ' ')
        , CHAR(13)
        , ' '
          )
          ) IN ('split', 'merge');
