-- 000036: Accounting System Optimization (QB/SAP/Xero alignment)
-- suppliers → vendors rename, multi-currency, tax codes, exchange rates, etc.

BEGIN;

-- ============================================================
-- 1. suppliers → vendors rename
-- ============================================================
ALTER TABLE suppliers RENAME TO vendors;
ALTER TABLE vendors RENAME COLUMN supplier_type TO vendor_type;

-- Rename indexes (IF EXISTS for safety)
ALTER INDEX IF EXISTS idx_suppliers_company RENAME TO idx_vendors_company;
ALTER INDEX IF EXISTS suppliers_pkey RENAME TO vendors_pkey;
ALTER INDEX IF EXISTS suppliers_company_id_tin_key RENAME TO vendors_company_id_tin_key;

-- Rename FK references in transactions
ALTER TABLE transactions RENAME COLUMN supplier_id TO vendor_id;

-- Rename FK references in withholding_certificates
ALTER TABLE withholding_certificates RENAME COLUMN supplier_id TO vendor_id;

-- Add new vendor fields
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS email VARCHAR(255);
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS phone VARCHAR(50);
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS payment_terms_days INT DEFAULT 30;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3);
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS default_account_id UUID REFERENCES accounts(id);
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE vendors ADD COLUMN IF NOT EXISTS notes TEXT;

-- ============================================================
-- 2. companies.base_currency
-- ============================================================
ALTER TABLE companies ADD COLUMN IF NOT EXISTS base_currency VARCHAR(3) NOT NULL DEFAULT 'PHP';
UPDATE companies SET base_currency = 'SGD' WHERE jurisdiction = 'SG';
UPDATE companies SET base_currency = 'LKR' WHERE jurisdiction = 'LK';

-- ============================================================
-- 3. accounts new columns
-- ============================================================
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS default_tax_code VARCHAR(20);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cash_flow_category VARCHAR(20); -- operating/investing/financing

-- ============================================================
-- 4. journal_entries + journal_lines multi-currency & tax
-- ============================================================
ALTER TABLE journal_entries ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3);

ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3);
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS exchange_rate NUMERIC(15,6);
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS home_debit NUMERIC(15,2) NOT NULL DEFAULT 0;
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS home_credit NUMERIC(15,2) NOT NULL DEFAULT 0;
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS tax_code VARCHAR(20);
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS tax_amount NUMERIC(15,2) DEFAULT 0;

-- ============================================================
-- 5. tax_codes reference table
-- ============================================================
CREATE TABLE IF NOT EXISTS tax_codes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    tax_type VARCHAR(20) NOT NULL, -- vat/ewt/gst/wht
    rate NUMERIC(5,4) NOT NULL,
    is_inclusive BOOLEAN NOT NULL DEFAULT false,
    affects_account_id UUID REFERENCES accounts(id),
    jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, code)
);

CREATE INDEX IF NOT EXISTS idx_tax_codes_company ON tax_codes(company_id);

-- ============================================================
-- 6. exchange_rates table
-- ============================================================
CREATE TABLE IF NOT EXISTS exchange_rates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    from_currency VARCHAR(3) NOT NULL,
    to_currency VARCHAR(3) NOT NULL,
    rate NUMERIC(15,6) NOT NULL,
    effective_date DATE NOT NULL,
    source VARCHAR(20) DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, from_currency, to_currency, effective_date)
);

CREATE INDEX IF NOT EXISTS idx_exchange_rates_company ON exchange_rates(company_id);
CREATE INDEX IF NOT EXISTS idx_exchange_rates_lookup ON exchange_rates(company_id, from_currency, to_currency, effective_date DESC);

COMMIT;
