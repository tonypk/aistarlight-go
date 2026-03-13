DROP INDEX IF EXISTS idx_rb_image_hash;
ALTER TABLE receipt_batches DROP COLUMN IF EXISTS image_hash;
