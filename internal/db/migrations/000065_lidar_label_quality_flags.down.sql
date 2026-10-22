-- The previous version already reads these quality flags. Do not turn a flag
-- back into an object class: there is no ambiguity to restore on rollback.
   SELECT 1;
