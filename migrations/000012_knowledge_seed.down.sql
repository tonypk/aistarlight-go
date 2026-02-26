-- Remove seeded knowledge chunks (by source pattern)
DELETE FROM knowledge_chunks WHERE source IN (
  'NIRC Section 106-108',
  'RR No. 16-2005',
  'NIRC Section 114',
  'RR No. 13-2018',
  'BIR Form 2550M Instructions',
  'RR No. 2-98 as amended',
  'BIR Form 0619-E Instructions',
  'RR No. 11-2018',
  'BIR Form 2307',
  'NIRC Section 24 (TRAIN Law)',
  'NIRC Section 27',
  'BIR Form 1701 Instructions',
  'BIR Form 1702 Instructions',
  'RR No. 7-2024',
  'NIRC Section 248-249',
  'NIRC Section 237',
  'RR No. 1-2014',
  'BIR Filing Calendar',
  'CREATE Law (RA 11534)'
);
