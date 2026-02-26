-- Add structured citation columns to knowledge_chunks
ALTER TABLE knowledge_chunks
  ADD COLUMN IF NOT EXISTS section_ref VARCHAR(100),
  ADD COLUMN IF NOT EXISTS law_ref VARCHAR(200),
  ADD COLUMN IF NOT EXISTS effective_date DATE,
  ADD COLUMN IF NOT EXISTS chunk_type VARCHAR(30) DEFAULT 'text';

-- Create indexes for filtered searches
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_category ON knowledge_chunks(category);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_chunk_type ON knowledge_chunks(chunk_type);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_section_ref ON knowledge_chunks(section_ref);

-- Backfill section_ref and law_ref from existing metadata JSONB
-- Extract section references from metadata->>'section' or source field
UPDATE knowledge_chunks
SET section_ref = COALESCE(
  metadata->>'section',
  CASE
    WHEN source ~ 'Section \d+' THEN (regexp_match(source, 'Section (\d+[A-Z]?)'))[1]
    WHEN source ~ 'Sec\. \d+' THEN (regexp_match(source, 'Sec\. (\d+[A-Z]?)'))[1]
    ELSE NULL
  END
)
WHERE section_ref IS NULL
  AND (metadata->>'section' IS NOT NULL OR source ~ '(Section|Sec\.) \d+');

-- Extract law references
UPDATE knowledge_chunks
SET law_ref = COALESCE(
  metadata->>'law',
  CASE
    WHEN source ILIKE '%NIRC%' THEN 'NIRC'
    WHEN source ILIKE '%RA %' OR source ILIKE '%Republic Act%' THEN (regexp_match(source, '(?:RA|Republic Act)\s*(?:No\.\s*)?(\d+)'))[1]
    WHEN source ILIKE '%RR %' OR source ILIKE '%Revenue Regulation%' THEN (regexp_match(source, '(?:RR|Revenue Regulation)\s*(?:No\.\s*)?([0-9-]+)'))[1]
    WHEN source ILIKE '%RMC%' THEN (regexp_match(source, 'RMC\s*(?:No\.\s*)?([0-9-]+)'))[1]
    ELSE NULL
  END
)
WHERE law_ref IS NULL
  AND (metadata->>'law' IS NOT NULL OR source ~ '(?:NIRC|RA |RR |RMC|Republic Act|Revenue Regulation)');

-- Set chunk_type based on category and content patterns
UPDATE knowledge_chunks
SET chunk_type = CASE
  WHEN category IN ('rate', 'rates', 'tax_rate') THEN 'rate_table'
  WHEN category IN ('penalty', 'penalties') THEN 'penalty'
  WHEN category IN ('form_instruction', 'instructions') THEN 'form_instruction'
  WHEN content ILIKE '%penalty%' OR content ILIKE '%surcharge%' THEN 'penalty'
  WHEN content ILIKE '%rate%' AND (content ILIKE '%percent%' OR content ILIKE '%%%') THEN 'rate_table'
  ELSE 'text'
END
WHERE chunk_type = 'text' OR chunk_type IS NULL;
