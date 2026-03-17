-- Electronic Invoice System (EIS) tables

CREATE TABLE invoices (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    invoice_number  VARCHAR(50) NOT NULL,
    invoice_type    VARCHAR(20) NOT NULL DEFAULT 'sales',  -- sales, purchase, credit_note, debit_note
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',  -- draft, issued, submitted, accepted, rejected, cancelled

    -- Parties
    customer_name   TEXT NOT NULL,
    customer_tin    VARCHAR(20),
    customer_address TEXT,

    -- Dates
    invoice_date    DATE NOT NULL,
    due_date        DATE,

    -- Amounts
    subtotal        NUMERIC(15,2) NOT NULL DEFAULT 0,
    vat_amount      NUMERIC(15,2) NOT NULL DEFAULT 0,
    discount_amount NUMERIC(15,2) NOT NULL DEFAULT 0,
    total_amount    NUMERIC(15,2) NOT NULL DEFAULT 0,

    -- VAT breakdown
    vatable_sales       NUMERIC(15,2) NOT NULL DEFAULT 0,
    vat_exempt_sales    NUMERIC(15,2) NOT NULL DEFAULT 0,
    zero_rated_sales    NUMERIC(15,2) NOT NULL DEFAULT 0,

    -- EIS fields
    eis_submission_id   VARCHAR(100),
    eis_status          VARCHAR(20),       -- pending, submitted, accepted, rejected
    eis_submitted_at    TIMESTAMPTZ,
    eis_response        JSONB,

    -- References
    reference_number    VARCHAR(50),
    notes               TEXT,
    vendor_id           UUID REFERENCES vendors(id),

    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE invoice_items (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    line_number     INT NOT NULL,
    description     TEXT NOT NULL,
    quantity        NUMERIC(10,3) NOT NULL DEFAULT 1,
    unit_price      NUMERIC(15,2) NOT NULL DEFAULT 0,
    amount          NUMERIC(15,2) NOT NULL DEFAULT 0,
    vat_type        VARCHAR(20) NOT NULL DEFAULT 'vatable', -- vatable, exempt, zero_rated
    vat_rate        NUMERIC(5,2) NOT NULL DEFAULT 12.00,
    vat_amount      NUMERIC(15,2) NOT NULL DEFAULT 0,
    discount        NUMERIC(15,2) NOT NULL DEFAULT 0,
    atc_code        VARCHAR(20),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_invoices_company_id ON invoices(company_id);
CREATE INDEX idx_invoices_company_date ON invoices(company_id, invoice_date DESC);
CREATE INDEX idx_invoices_status ON invoices(company_id, status);
CREATE INDEX idx_invoices_eis_status ON invoices(company_id, eis_status) WHERE eis_status IS NOT NULL;
CREATE INDEX idx_invoice_items_invoice_id ON invoice_items(invoice_id);

-- Unique invoice number per company
CREATE UNIQUE INDEX idx_invoices_unique_number ON invoices(company_id, invoice_number);
