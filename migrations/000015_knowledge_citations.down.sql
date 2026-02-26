DROP INDEX IF EXISTS idx_knowledge_chunks_section_ref;
DROP INDEX IF EXISTS idx_knowledge_chunks_chunk_type;
DROP INDEX IF EXISTS idx_knowledge_chunks_category;

ALTER TABLE knowledge_chunks
  DROP COLUMN IF EXISTS chunk_type,
  DROP COLUMN IF EXISTS effective_date,
  DROP COLUMN IF EXISTS law_ref,
  DROP COLUMN IF EXISTS section_ref;
