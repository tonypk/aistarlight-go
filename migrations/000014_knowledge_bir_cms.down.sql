-- Remove BIR Official CMS knowledge chunks added in migration 014
DELETE FROM knowledge_chunks WHERE source LIKE 'BIR Official -%';
