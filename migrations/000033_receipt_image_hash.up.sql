-- Add image hash column for duplicate image detection.
ALTER TABLE receipt_batches
    ADD COLUMN IF NOT EXISTS image_hash VARCHAR(64);

CREATE INDEX idx_rb_image_hash ON receipt_batches(company_id, image_hash)
    WHERE image_hash IS NOT NULL;
