-- Remove Singapore IRAS knowledge chunks
DELETE FROM knowledge_chunks WHERE jurisdiction = 'SG';
