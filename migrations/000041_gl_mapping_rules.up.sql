-- GL mapping rules: map payroll dimensions to chart-of-accounts entries.
CREATE TABLE gl_mapping_rules (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       UUID        NOT NULL REFERENCES companies(id),
    jurisdiction     VARCHAR(3)  NOT NULL DEFAULT 'PH',
    source_dimension VARCHAR(50) NOT NULL,
    source_value     VARCHAR(100) NOT NULL,
    target_account_id UUID       NOT NULL REFERENCES accounts(id),
    debit_credit     VARCHAR(6)  NOT NULL CHECK (debit_credit IN ('debit', 'credit')),
    priority         INT         NOT NULL DEFAULT 0,
    effective_from   DATE        NOT NULL DEFAULT CURRENT_DATE,
    effective_to     DATE,
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, jurisdiction, source_dimension, source_value, effective_from)
);

CREATE INDEX idx_gl_mapping_active
    ON gl_mapping_rules(company_id, jurisdiction, is_active)
    WHERE is_active = TRUE;

-- HR payees: employees synced from AIGoNHR as counterparties for tax forms.
-- Separate from suppliers to avoid altering the legacy table.
CREATE TABLE hr_payees (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID         NOT NULL REFERENCES companies(id),
    hr_employee_id  BIGINT       NOT NULL,
    hr_employee_no  VARCHAR(50)  NOT NULL,
    first_name      VARCHAR(100) NOT NULL,
    last_name       VARCHAR(100) NOT NULL,
    email           VARCHAR(200),
    tin             VARCHAR(20),
    sss             VARCHAR(20),
    philhealth      VARCHAR(20),
    pagibig         VARCHAR(20),
    department_name VARCHAR(200),
    position_title  VARCHAR(200),
    jurisdiction    VARCHAR(3)   NOT NULL DEFAULT 'PH',
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, hr_employee_id)
);

CREATE INDEX idx_hr_payees_company
    ON hr_payees(company_id, status);
